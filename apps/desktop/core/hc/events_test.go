package hc

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestSubscriptionOrderAndMissed(t *testing.T) {
	c, _ := newTestConn(t, func(f *fakeCore, fr frame) {
		req, ok := requestID(t, fr)
		if !ok || req.Method != "events.subscribe" {
			return
		}
		f.result(req.ID, map[string]any{
			"channel":   7,
			"seq":       2,
			"terminals": []Terminal{{Address: "t1", State: "live"}},
		})

		line := func(seq uint64, typ, terminal string, data string) []byte {
			return []byte(`{"jsonrpc":"2.0","method":"event","params":{"seq":` +
				strconv.FormatUint(seq, 10) + `,"type":"` + typ + `","terminal":"` + terminal + `","data":` + data + `}}` + "\n")
		}
		first := line(3, "status", "t1", `{"state":"working"}`)
		f.write(7, kindData, first)
		f.write(7, kindData, line(2, "title", "t1", `{"title":"old"}`))
		split := line(4, "title", "t1", `{"title":"new"}`)
		f.write(7, kindData, split[:20])
		f.write(7, kindData, split[20:])
		f.write(7, kindData, []byte(`{"jsonrpc":"2.0","method":"missed","params":{"seq":5,"terminals":[{"address":"t9","state":"live"}]}}`+"\n"))
		f.write(7, kindData, line(6, "size", "t1", `{"cols":100,"rows":40}`))
		f.write(7, kindClose, []byte(`{"reason":"exited","exit_code":0}`))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	sub, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sub.Snapshot.Seq != 2 || len(sub.Snapshot.Terminals) != 1 || sub.Snapshot.Terminals[0].Address != "t1" {
		t.Fatalf("snapshot = %+v", sub.Snapshot)
	}

	var got []Event
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-sub.Events:
			if !ok {
				goto done
			}
			got = append(got, ev)
		case <-deadline:
			t.Fatalf("timed out; got %d events", len(got))
		}
	}
done:
	if len(got) != 4 {
		t.Fatalf("got %d events: %+v", len(got), got)
	}
	if got[0].Type != "status" || got[0].Seq != 3 {
		t.Fatalf("first = %+v", got[0])
	}
	if got[1].Type != "title" || got[1].Seq != 4 {
		t.Fatalf("second = %+v", got[1])
	}
	if got[2].Type != "missed" {
		t.Fatalf("third = %+v", got[2])
	}
	var snap Snapshot
	if err := json.Unmarshal(got[2].Data, &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Seq != 5 || len(snap.Terminals) != 1 || snap.Terminals[0].Address != "t9" {
		t.Fatalf("missed data = %s", got[2].Data)
	}
	if got[3].Type != "size" || got[3].Seq != 6 {
		t.Fatalf("fourth = %+v", got[3])
	}
}

func TestSubscriptionClose(t *testing.T) {
	c, fc := newTestConn(t, func(f *fakeCore, fr frame) {
		req, ok := requestID(t, fr)
		if !ok || req.Method != "events.subscribe" {
			return
		}
		f.result(req.ID, map[string]any{"channel": 8, "seq": 1, "terminals": []Terminal{}})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	sub, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sub.Close()
	fc.waitFrame(t, func(f frame) bool {
		return f.channel == 8 && f.kind == kindClose
	})
	select {
	case _, ok := <-sub.Events:
		if ok {
			t.Fatal("event channel stayed open")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event channel did not close")
	}
	// Close is idempotent.
	sub.Close()
}
