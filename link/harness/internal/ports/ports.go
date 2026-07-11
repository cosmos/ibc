// Package ports allocates dynamic, OS-assigned TCP ports so the harness never hard-codes a port.
package ports

import (
	"fmt"
	"net"
	"sync"
)

var (
	mu     sync.Mutex
	issued = map[int]bool{}
)

// FreePort reserves each returned port within the process, but callers must bind promptly to avoid host races.
func FreePort() (int, error) {
	mu.Lock()
	defer mu.Unlock()
	for {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, fmt.Errorf("listen for free port: %w", err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
		if issued[port] {
			continue
		}
		issued[port] = true
		return port, nil
	}
}
