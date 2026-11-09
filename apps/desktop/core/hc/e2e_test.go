package hc

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestE2E runs against a real core in the hiik-test session. It never touches
// the user's default session: the session name and home are set here.
func TestE2E(t *testing.T) {
	if os.Getenv("HIIK_E2E") != "1" {
		t.Skip("set HIIK_E2E=1 to run against a real core")
	}
	if _, err := exec.LookPath("hc"); err != nil {
		t.Skip("hc is not in PATH")
	}
	t.Setenv("HC_SESSION", "hiik-test")
	t.Setenv("HIIK_HOME", t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := Dial(ctx, Options{Session: "hiik-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.Hello().Generation != 1 || client.Hello().Version == "" {
		t.Fatalf("hello = %+v", client.Hello())
	}

	term, err := client.Run(ctx, RunRequest{
		Argv:   []string{"/bin/sh"},
		Labels: map[string]string{"hiik.e2e": "1"},
		Cols:   80,
		Rows:   24,
	})
	if err != nil {
		t.Fatal(err)
	}
	killed := false
	defer func() {
		if !killed {
			kctx, kcancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer kcancel()
			_ = client.Kill(kctx, term.Address)
		}
	}()

	at, err := client.Attach(ctx, AttachRequest{
		Terminal: term.Address,
		Cols:     80,
		Rows:     24,
		History:  200,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer at.Close()

	if _, err := at.Conn.Write([]byte("echo hi\n")); err != nil {
		t.Fatal(err)
	}

	readCh := make(chan string, 1)
	go func() {
		var out bytes.Buffer
		buf := make([]byte, 4096)
		for {
			n, err := at.Conn.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
				if strings.Contains(out.String(), "hi") {
					readCh <- out.String()
					return
				}
			}
			if err != nil {
				readCh <- out.String()
				return
			}
		}
	}()
	select {
	case got := <-readCh:
		if !strings.Contains(got, "hi") {
			t.Fatalf("terminal output = %q", got)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for terminal output")
	}

	resizer, ok := at.Conn.(interface{ Resize(int, int) error })
	if !ok {
		t.Fatal("connection has no Resize")
	}
	if err := resizer.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	if err := at.Close(); err != nil {
		t.Fatal(err)
	}

	kctx, kcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer kcancel()
	if err := client.Kill(kctx, term.Address); err != nil {
		t.Fatal(err)
	}
	killed = true
}
