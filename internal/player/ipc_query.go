package player

import (
	"errors"
	"net"
	"time"
)

// ipcQueryTimeout bounds a single vo-configured query on a platform whose
// connection cannot enforce a read deadline. Windows named-pipe handles opened
// through os.OpenFile are synchronous, so the deadline queryVoConfigured sets
// is inert there; without an outer bound a wedged pipe would block the caller
// and the UI goroutine that waits on playback forever. It doubles as the
// default bound of queryVoConfiguredBounded.
const ipcQueryTimeout = 750 * time.Millisecond

// errIPCQueryTimeout reports that mpv did not answer a query in time.
var errIPCQueryTimeout = errors.New("mpv ipc query timed out")

// queryVoConfiguredBounded runs the shared vo-configured query with an outer
// bound. The caller is expected to close the connection when this returns an
// error, which unblocks the pending read so the helper goroutine cannot
// outlive the poll iteration.
func queryVoConfiguredBounded(conn net.Conn, timeout time.Duration) (bool, error) {
	// A missing bound falls back to ipcQueryTimeout, the same value the Windows
	// poll loop passes explicitly, so a wedged pipe can never block forever.
	if timeout <= 0 {
		timeout = ipcQueryTimeout
	}

	type queryResult struct {
		confirmed bool
		err       error
	}

	results := make(chan queryResult, 1)
	go func() {
		confirmed, err := queryVoConfigured(conn)
		results <- queryResult{confirmed: confirmed, err: err}
	}()

	select {
	case result := <-results:
		return result.confirmed, result.err
	case <-time.After(timeout):
		return false, errIPCQueryTimeout
	}
}
