package hc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
)

func TestAttachDataCreditResizeClose(t *testing.T) {
	paramsCh := make(chan map[string]any, 4)
	attachCount := 0
	c, fc := newTestConn(t, func(f *fakeCore, fr frame) {
		req, ok := requestID(t, fr)
		if !ok || req.Method != "terminal.attach" {
			return
		}
		attachCount++
		var params map[string]any
		_ = json.Unmarshal(req.Params, &params)
		paramsCh <- params
		if attachCount == 1 {
			f.result(req.ID, map[string]any{
				"channel":              4,
				"terminal":             Terminal{Address: "t1", State: "live"},
				"resize_notifications": true,
				"automatic_input":      true,
			})
			f.write(4, kindData, []byte("hello"))
		} else {
			f.result(req.ID, map[string]any{
				"channel":         5,
				"terminal":        Terminal{Address: "t1", State: "live"},
				"automatic_input": true,
			})
			f.write(5, kindClose, []byte(`{"reason":"exited","exit_code":0}`))
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	at, err := c.Attach(ctx, AttachRequest{
		Terminal: "t1",
		Cols:     80,
		Rows:     24,
		History:  2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if at.Channel != 4 || at.Terminal.Address != "t1" || at.Conn == nil {
		t.Fatalf("attachment = %+v", at)
	}
	params := <-paramsCh
	if params["terminal"] != "t1" || params["cols"] != float64(80) || params["rows"] != float64(24) {
		t.Fatalf("attach params = %+v", params)
	}
	if params["resize_notifications"] != true || params["observe"] != false {
		t.Fatalf("attach flags = %+v", params)
	}

	credit := fc.waitFrame(t, func(f frame) bool {
		return f.channel == 4 && f.kind == kindCredit
	})
	if got := binary.LittleEndian.Uint32(credit.payload); got != initialCredit {
		t.Fatalf("initial credit = %d, want %d", got, initialCredit)
	}

	buf := make([]byte, 5)
	if _, err := io.ReadFull(at.Conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "hello" {
		t.Fatalf("read %q", buf)
	}
	credit = fc.waitFrame(t, func(f frame) bool {
		return f.channel == 4 && f.kind == kindCredit
	})
	if got := binary.LittleEndian.Uint32(credit.payload); got != 5 {
		t.Fatalf("read credit = %d, want 5", got)
	}

	if n, err := at.Conn.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("write = %d, %v", n, err)
	}
	data := fc.waitFrame(t, func(f frame) bool {
		return f.channel == 4 && f.kind == kindData
	})
	if string(data.payload) != "abc" {
		t.Fatalf("data = %q", data.payload)
	}

	resizer := at.Conn.(interface{ Resize(int, int) error })
	if err := resizer.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	resize := fc.waitFrame(t, func(f frame) bool {
		return f.channel == 4 && f.kind == kindResize
	})
	if got := binary.LittleEndian.Uint16(resize.payload[0:2]); got != 100 {
		t.Fatalf("resize cols = %d", got)
	}
	if got := binary.LittleEndian.Uint16(resize.payload[2:4]); got != 30 {
		t.Fatalf("resize rows = %d", got)
	}

	themed := at.Conn.(interface{ UseTheme(*terminal.Theme) error })
	if err := themed.UseTheme(terminal.DarkTheme()); err != nil {
		t.Fatal(err)
	}
	facts := fc.waitFrame(t, func(f frame) bool {
		return f.channel == 4 && f.kind == kindFacts
	})
	var display map[string]any
	if err := json.Unmarshal(facts.payload, &display); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"bg", "fg", "cursor"} {
		v, _ := display[key].(string)
		if len(v) != 7 || v[0] != '#' {
			t.Fatalf("%s = %q in %s", key, v, facts.payload)
		}
	}

	pixeled := at.Conn.(interface {
		ResizePixels(cols, rows, cellW, cellH int) error
	})
	if err := pixeled.ResizePixels(120, 40, 8, 16); err != nil {
		t.Fatal(err)
	}
	fc.waitFrame(t, func(f frame) bool {
		return f.channel == 4 && f.kind == kindResize
	})
	facts = fc.waitFrame(t, func(f frame) bool {
		return f.channel == 4 && f.kind == kindFacts
	})
	if err := json.Unmarshal(facts.payload, &display); err != nil {
		t.Fatal(err)
	}
	cell, _ := display["cell"].([]any)
	if len(cell) != 2 || cell[0] != float64(8) || cell[1] != float64(16) {
		t.Fatalf("cell = %+v in %s", display["cell"], facts.payload)
	}

	if err := at.Close(); err != nil {
		t.Fatal(err)
	}
	fc.waitFrame(t, func(f frame) bool {
		return f.channel == 4 && f.kind == kindClose
	})
	if _, err := at.Conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("read after close = %v", err)
	}
	if err := at.Conn.Close(); err != nil {
		t.Fatalf("second close = %v", err)
	}

	// A CLOSE frame from the core ends Read with io.EOF.
	at2, err := c.Attach(ctx, AttachRequest{Terminal: "t1", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := at2.Conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("core close read = %v", err)
	}
}
