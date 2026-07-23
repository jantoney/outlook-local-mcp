package webui

import (
	"fmt"
	"net"
)

// Listen binds the configured port on IPv4 loopback only. It returns the
// listener and canonical URL on success. Bind failures are returned so startup
// can disable the UI while continuing the stdio MCP transport.
func Listen(port int) (net.Listener, string, error) {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		return nil, "", fmt.Errorf("listen on %s: %w", address, err)
	}
	return listener, "http://" + address, nil
}
