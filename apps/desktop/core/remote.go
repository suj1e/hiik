package core

// Remote access for panels: one program or one text file on a machine the
// core reaches. Both go through the core's RPC once a machine is named; an
// empty machine runs here. The 15 second context bounds the whole call so a
// panel never waits on an unreachable host.

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	// remoteCallTimeout bounds one machine.exec or file.read.
	remoteCallTimeout = 15 * time.Second
	// remoteTextBytes is how much text ReadTextFile returns.
	remoteTextBytes = 1 << 20
)

// execResult is the core's machine.exec result. Output is base64 because a
// program may write arbitrary bytes.
type execResult struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Code      *int   `json:"code"`
	Truncated bool   `json:"truncated"`
	TimedOut  bool   `json:"timed_out"`
	MS        uint64 `json:"ms"`
}

// fileReadResult is the part of the core's file.read result this app uses.
type fileReadResult struct {
	Text string `json:"text"`
}

// Exec runs one program on a machine. A remote run calls the core's
// machine.exec with exactly the command and arguments given; its stdout,
// stderr and exit code come back unchanged. A nonzero exit is a result,
// not an error.
func (a *App) Exec(on, command string, args ...string) (string, string, int, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", "", -1, errors.New("exec: empty command")
	}
	if on == "" {
		return execLocal(command, args...)
	}
	c, err := a.coreClient()
	if err != nil {
		return "", "", -1, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteCallTimeout)
	defer cancel()

	argv := make([]string, 0, len(args)+1)
	argv = append(argv, command)
	argv = append(argv, args...)
	params := map[string]any{
		"argv":    argv,
		"on":      on,
		"timeout": remoteCallTimeout.Seconds(),
	}
	var res execResult
	if err := c.Call(ctx, "machine.exec", params, &res); err != nil {
		return "", "", -1, fmt.Errorf("machine.exec on %s: %w", on, err)
	}
	stdout, err := decodeExecOutput(res.Stdout)
	if err != nil {
		return "", "", -1, fmt.Errorf("machine.exec on %s: stdout: %w", on, err)
	}
	stderr, err := decodeExecOutput(res.Stderr)
	if err != nil {
		return "", "", -1, fmt.Errorf("machine.exec on %s: stderr: %w", on, err)
	}
	code := -1
	if res.Code != nil {
		code = *res.Code
	}
	if res.TimedOut {
		return string(stdout), string(stderr), code,
			fmt.Errorf("machine.exec on %s timed out after %s", on, remoteCallTimeout)
	}
	return string(stdout), string(stderr), code, nil
}

// execLocal runs a program on this computer with the same deadline.
func execLocal(command string, args ...string) (string, string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), remoteCallTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return stdout.String(), stderr.String(), exit.ExitCode(), nil
	}
	if ctx.Err() == context.DeadlineExceeded {
		return stdout.String(), stderr.String(), -1,
			fmt.Errorf("%s timed out after %s", command, remoteCallTimeout)
	}
	return stdout.String(), stderr.String(), -1, err
}

// ReadTextFile reads a text file on a machine. A remote read calls the
// core's file.read; the result is capped at 1 MiB.
func (a *App) ReadTextFile(path, on string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("read: empty path")
	}
	if on == "" {
		return readLocalText(path)
	}
	c, err := a.coreClient()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteCallTimeout)
	defer cancel()

	params := map[string]any{"path": path, "on": on}
	var res fileReadResult
	if err := c.Call(ctx, "file.read", params, &res); err != nil {
		return "", fmt.Errorf("file.read on %s: %w", on, err)
	}
	return capText(res.Text), nil
}

// readLocalText reads at most remoteTextBytes plus one byte so the caller
// can tell a file at the cap from a longer one.
func readLocalText(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, remoteTextBytes+1))
	if err != nil {
		return "", err
	}
	return capText(string(data)), nil
}

// capText keeps the first 1 MiB and drops a partial trailing rune.
func capText(s string) string {
	if len(s) <= remoteTextBytes {
		return s
	}
	return strings.ToValidUTF8(s[:remoteTextBytes], "")
}

func decodeExecOutput(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(s)
}
