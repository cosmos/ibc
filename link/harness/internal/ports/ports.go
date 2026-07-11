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

// FreePort asks the kernel for an unused TCP port by binding :0, reads back the assignment, then
// releases it. Allocation is race-safe across concurrent callers: a process-wide set records every
// port handed out, so two overlapping launches never receive the same one. The kernel only
// guarantees distinctness while both listeners are open; the set covers the window after release,
// before the subprocess binds — the case concurrent chain launches would otherwise hit. The
// remaining race with unrelated host processes is inherent to :0 allocation, so the consumer should
// still bind immediately.
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
