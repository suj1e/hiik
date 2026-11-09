package core

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"hiik/core/hc"
)

// fakeCore is a scripted hc.Client for model and command tests.
type fakeCore struct {
	mu     sync.Mutex
	hello  hc.Hello
	runs   []hc.RunRequest
	killed []string
	next   int
	closed bool

	runErr   error
	killErr  error
	labelErr error
	labels   []labelCall
}

// labelCall is one terminal.label request the app made.
type labelCall struct {
	Address string
	Labels  map[string]string
}

func newFakeCore() *fakeCore {
	return &fakeCore{hello: hc.Hello{Generation: 2, Version: "0.1.0"}}
}

func (f *fakeCore) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

func (f *fakeCore) Hello() hc.Hello {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hello
}

func (f *fakeCore) Call(context.Context, string, any, any) error { return nil }

func (f *fakeCore) Subscribe(context.Context) (*hc.Subscription, error) {
	return nil, errors.New("fake: no subscription")
}

func (f *fakeCore) List(context.Context, bool) ([]hc.Terminal, error) { return nil, nil }

func (f *fakeCore) Run(_ context.Context, req hc.RunRequest) (hc.Terminal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.runErr != nil {
		return hc.Terminal{}, f.runErr
	}
	f.runs = append(f.runs, req)
	f.next++
	return hc.Terminal{Address: fmt.Sprintf("t%d", f.next), State: "live"}, nil
}

func (f *fakeCore) Kill(_ context.Context, address string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.killErr != nil {
		return f.killErr
	}
	f.killed = append(f.killed, address)
	return nil
}

func (f *fakeCore) Label(_ context.Context, address string, labels map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.labelErr != nil {
		return f.labelErr
	}
	copied := make(map[string]string, len(labels))
	for k, v := range labels {
		copied[k] = v
	}
	f.labels = append(f.labels, labelCall{Address: address, Labels: copied})
	return nil
}

func (f *fakeCore) labelsSnapshot() []labelCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]labelCall(nil), f.labels...)
}

func (f *fakeCore) Attach(context.Context, hc.AttachRequest) (*hc.Attachment, error) {
	return nil, errors.New("fake: no attach")
}

func (f *fakeCore) Lost() <-chan struct{} { return nil }

func (f *fakeCore) runsSnapshot() []hc.RunRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]hc.RunRequest(nil), f.runs...)
}

func (f *fakeCore) killedSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.killed...)
}

// testApp returns an app with no store, so tests never touch the user's
// config directory.
func testApp() *App {
	return newApp(nil)
}
