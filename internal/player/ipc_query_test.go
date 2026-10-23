package player

import (
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// scriptedConn is a net.Conn test double. Reads return the queued script lines
// and then block until the connection is closed, which models a Windows named
// pipe: a synchronous handle whose SetDeadline is silently ignored.
type scriptedConn struct {
	mu        sync.Mutex
	pending   []string
	closed    chan struct{}
	closeOnce sync.Once
}

func newScriptedConn(lines ...string) *scriptedConn {
	return &scriptedConn{pending: lines, closed: make(chan struct{})}
}

func (c *scriptedConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	if len(c.pending) > 0 {
		line := c.pending[0]
		c.pending = c.pending[1:]
		c.mu.Unlock()
		return copy(p, line), nil
	}
	c.mu.Unlock()

	<-c.closed
	return 0, net.ErrClosed
}

func (c *scriptedConn) Write(p []byte) (int, error) { return len(p), nil }

func (c *scriptedConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *scriptedConn) LocalAddr() net.Addr  { return testAddr("local") }
func (c *scriptedConn) RemoteAddr() net.Addr { return testAddr("remote") }

// SetDeadline is a no-op on purpose: a synchronous pipe cannot honour it.
func (c *scriptedConn) SetDeadline(time.Time) error      { return nil }
func (c *scriptedConn) SetReadDeadline(time.Time) error  { return nil }
func (c *scriptedConn) SetWriteDeadline(time.Time) error { return nil }

type testAddr string

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return string(a) }

// TestQueryVoConfiguredBoundedReturnsAnswer covers the healthy path: mpv
// answers vo-configured and the bounded query reports the confirmation.
func TestQueryVoConfiguredBoundedReturnsAnswer(t *testing.T) {
	conn := newScriptedConn(`{"data":true,"error":"success"}` + "\n")
	// Scripted stub: closing it is cleanup only, no state is persisted.
	defer func() { _ = conn.Close() }()

	confirmed, err := queryVoConfiguredBounded(conn, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !confirmed {
		t.Fatalf("expected the confirmation to be reported")
	}
}

// TestQueryVoConfiguredBoundedTimesOut proves a pipe that never answers cannot
// block the poll loop: the bounded query returns the timeout error instead of
// hanging on a silent read.
func TestQueryVoConfiguredBoundedTimesOut(t *testing.T) {
	conn := newScriptedConn() // never answers
	// Scripted stub: closing it is cleanup only, no state is persisted.
	defer func() { _ = conn.Close() }()

	type outcome struct {
		confirmed bool
		err       error
		elapsed   time.Duration
	}

	start := time.Now()
	done := make(chan outcome, 1)
	go func() {
		confirmed, err := queryVoConfiguredBounded(conn, 100*time.Millisecond)
		done <- outcome{confirmed: confirmed, err: err, elapsed: time.Since(start)}
	}()

	// The outer guard makes an unbounded regression fail fast instead of
	// hanging until the package test timeout.
	var got outcome
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("bounded query did not return within 2s; it is no longer bounded")
	}

	if !errors.Is(got.err, errIPCQueryTimeout) {
		t.Fatalf("expected errIPCQueryTimeout, got %v", got.err)
	}
	if got.confirmed {
		t.Fatalf("a timed-out query must not report a confirmation")
	}
	if got.elapsed > 2*time.Second {
		t.Fatalf("bounded query took %s, expected roughly the supplied timeout", got.elapsed)
	}
}

// TestQueryVoConfiguredBoundedIgnoresIgnoredDeadline locks the reason the
// outer bound exists: the connection's own deadline does nothing, so the answer
// still has to arrive through the scripted read.
func TestQueryVoConfiguredBoundedIgnoresIgnoredDeadline(t *testing.T) {
	conn := newScriptedConn(`{"data":false,"error":"success"}` + "\n")
	// Scripted stub: closing it is cleanup only, no state is persisted.
	defer func() { _ = conn.Close() }()

	confirmed, err := queryVoConfiguredBounded(conn, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if confirmed {
		t.Fatalf("an unavailable property must not be reported as confirmed")
	}
	if err := conn.SetDeadline(time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("SetDeadline stub must stay a no-op, got %v", err)
	}
}

// TestQueryVoConfiguredBoundedWritesCommand proves the query still sends the
// mpv command line the real IPC protocol expects.
func TestQueryVoConfiguredBoundedWritesCommand(t *testing.T) {
	recorder := &recordingConn{scriptedConn: newScriptedConn(`{"data":false,"error":"success"}` + "\n")}
	// Scripted stub: closing it is cleanup only, no state is persisted.
	defer func() { _ = recorder.Close() }()

	if _, err := queryVoConfiguredBounded(recorder, time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recorder.mu.Lock()
	written := recorder.written.String()
	recorder.mu.Unlock()

	if !strings.Contains(written, `"get_property"`) || !strings.Contains(written, `"vo-configured"`) {
		t.Fatalf("expected the vo-configured command on the wire, got %q", written)
	}
	if !strings.HasSuffix(written, "\n") {
		t.Fatalf("mpv IPC requires a newline-terminated command, got %q", written)
	}
}

type recordingConn struct {
	*scriptedConn
	written strings.Builder
}

func (c *recordingConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.written.Write(p)
	c.mu.Unlock()
	return len(p), nil
}
