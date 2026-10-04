package mqtt

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dnspatch/dnspatch/plugin"
)

// syncBuffer is a log destination that the test and the client's goroutines can
// use at the same time.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.String()
}

// connectedNotifier builds a notifier against b with a logger that writes to the
// returned buffer, and connects it.
func connectedNotifier(t *testing.T, b *fakeBroker) (plugin.Notifier, *syncBuffer) {
	t.Helper()

	n, err := build(t, map[string]any{"address": b.url()})
	if err != nil {
		t.Fatalf("BuildNotifier: %v", err)
	}

	logs := &syncBuffer{}
	n.(plugin.LoggerSetter).SetLogger(slog.New(slog.NewTextHandler(logs, nil)))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := n.(plugin.Connector).Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	return n, logs
}

func TestADroppedConnectionIsLoggedAtOnce(t *testing.T) {
	b := newFakeBroker(t)
	n, logs := connectedNotifier(t, b)

	t.Cleanup(func() { _ = n.Close() })

	b.dropConnections()

	deadline := time.Now().Add(5 * time.Second)

	for !strings.Contains(logs.String(), "lost its broker connection") {
		if time.Now().After(deadline) {
			t.Fatalf("nothing was logged after the broker dropped the connection, log: %q", logs.String())
		}

		time.Sleep(10 * time.Millisecond)
	}

	if !strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("log = %q, want an error", logs.String())
	}
}

func TestCloseIsNotLoggedAsALostConnection(t *testing.T) {
	b := newFakeBroker(t)
	n, logs := connectedNotifier(t, b)

	if err := n.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The client reports a loss asynchronously, so give a false one time to show.
	time.Sleep(300 * time.Millisecond)

	if got := logs.String(); got != "" {
		t.Errorf("Close logged %q, want nothing", got)
	}
}

func TestReconnectingIsNotLoggedAsALostConnection(t *testing.T) {
	b := newFakeBroker(t)
	n, logs := connectedNotifier(t, b)

	t.Cleanup(func() { _ = n.Close() })

	// A publish that the context cuts short drops the connection by itself and
	// opens a new one on the next call; neither is a loss the broker caused.
	b.silent.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	_ = n.Publish(ctx, "home", nil)

	cancel()
	b.silent.Store(false)

	if err := n.Publish(context.Background(), "home", nil); err != nil {
		t.Fatalf("Publish after the reconnect: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	if got := logs.String(); got != "" {
		t.Errorf("reconnecting logged %q, want nothing", got)
	}
}
