package hc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
)

// subscribeResult is the reply to events.subscribe.
type subscribeResult struct {
	Channel   *uint32    `json:"channel"`
	Seq       uint64     `json:"seq"`
	Terminals []Terminal `json:"terminals"`
}

// Subscribe opens the event stream and returns its snapshot plus a channel
// of ordered events.
func (c *conn) Subscribe(ctx context.Context) (*Subscription, error) {
	var res subscribeResult
	if err := c.Call(ctx, "events.subscribe", map[string]any{}, &res); err != nil {
		return nil, err
	}
	if res.Channel == nil {
		return nil, errors.New("hc: events.subscribe returned no channel; update hc")
	}
	channel := *res.Channel
	if channel == 0 {
		return nil, errors.New("hc: events.subscribe returned channel 0; update hc")
	}
	st := c.newStream(channel)
	events := make(chan Event, 128)
	done := make(chan struct{})

	var once sync.Once
	closeSub := func() {
		once.Do(func() {
			// Stop the consumer and close its channel before the
			// (possibly blocking) detach write.
			close(done)
			st.closeLocal()
			_ = c.writeFrame(channel, kindClose, nil)
			c.removeStream(channel)
		})
	}

	sub := &Subscription{
		Snapshot: Snapshot{
			Seq:       res.Seq,
			Terminals: res.Terminals,
		},
		Events: events,
		Close:  closeSub,
	}
	go c.eventLoop(st, channel, res.Seq, events, done)
	return sub, nil
}

// eventLoop turns newline-delimited JSON into ordered Events.
func (c *conn) eventLoop(st *stream, channel uint32, seq uint64, out chan<- Event, done <-chan struct{}) {
	defer close(out)
	defer c.removeStream(channel)
	var buf []byte
	for {
		f, err := st.next()
		if err != nil {
			return
		}
		if f.kind == kindClose {
			return
		}
		if f.kind != kindData {
			continue
		}
		consumed := len(f.payload)
		buf = append(buf, f.payload...)
		for {
			i := bytes.IndexByte(buf, '\n')
			if i < 0 {
				break
			}
			line := buf[:i]
			buf = append(buf[:0], buf[i+1:]...)
			if ev, next, ok := parseEvent(line, seq); ok {
				seq = next
				select {
				case out <- ev:
				case <-done:
					return
				case <-c.lost:
					return
				}
			}
		}
		// Credit is reported for the bytes consumed, independent of how
		// many complete lines they contained.
		_ = c.sendCredit(channel, consumed)
	}
}

// parseEvent parses one notification line. Events at or below seq are
// duplicates and are ignored. A missed snapshot advances seq and is
// delivered with its JSON snapshot as Data.
func parseEvent(line []byte, seq uint64) (Event, uint64, bool) {
	var msg rpcMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return Event{}, seq, false
	}
	switch msg.Method {
	case "event":
		var ev Event
		if err := json.Unmarshal(msg.Params, &ev); err != nil {
			return Event{}, seq, false
		}
		if ev.Seq <= seq {
			return Event{}, seq, false
		}
		if ev.Data == nil {
			ev.Data = json.RawMessage("null")
		} else {
			ev.Data = append([]byte(nil), ev.Data...)
		}
		return ev, ev.Seq, true
	case "missed":
		var snap struct {
			Seq       uint64     `json:"seq"`
			Terminals []Terminal `json:"terminals"`
		}
		if err := json.Unmarshal(msg.Params, &snap); err != nil {
			return Event{}, seq, false
		}
		data := append([]byte(nil), msg.Params...)
		return Event{Type: "missed", Data: data}, snap.Seq, true
	case "heartbeat":
		var beat struct {
			Seq uint64 `json:"seq"`
		}
		if err := json.Unmarshal(msg.Params, &beat); err == nil && beat.Seq > seq {
			seq = beat.Seq
		}
		return Event{}, seq, false
	default:
		return Event{}, seq, false
	}
}
