package instancebroker

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// Serve runs handler on the already elected listener until ctx is canceled or
// every proxy lease has been idle for the configured grace. It gracefully
// drains HTTP requests and returns unexpected transport errors.
func Serve(ctx context.Context, listener net.Listener, handler http.Handler, leases *Leases, shutdownTimeout time.Duration) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-serveErrors:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
			goto shutdown
		case now := <-ticker.C:
			if leases.Idle(now) {
				goto shutdown
			}
		}
	}

shutdown:
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
