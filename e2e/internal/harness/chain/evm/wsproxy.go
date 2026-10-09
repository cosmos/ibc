// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// WebSocketProxy forwards TCP between a local listener and an upstream ws:// host:port.
// Kill() closes active connections and rejects new ones until Revive() is called.
type WebSocketProxy struct {
	t testing.TB

	source   string
	listener net.Listener
	conns    []net.Conn
	blocked  bool

	mu sync.Mutex
}

func NewWebSocketProxy(t testing.TB, source string) *WebSocketProxy {
	t.Helper()

	hostPort := strings.TrimPrefix(source, "ws://")
	hostPort = strings.TrimPrefix(hostPort, "wss://")

	// allocate an available port on localhost
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	p := &WebSocketProxy{
		t:        t,
		source:   hostPort,
		listener: listener,
	}

	t.Cleanup(func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		p.blocked = true

		for _, conn := range p.conns {
			_ = conn.Close()
		}

		p.conns = nil
		_ = listener.Close()
	})

	go p.acceptLoop()

	return p
}

func (p *WebSocketProxy) URL() string {
	return "ws://" + p.listener.Addr().String()
}

func (p *WebSocketProxy) Kill() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.blocked {
		return
	}

	for _, conn := range p.conns {
		_ = conn.Close()
	}

	p.blocked = true

	p.t.Logf("WebSocketProxy %s killed", p.URL())
}

func (p *WebSocketProxy) Revive() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.blocked {
		return
	}

	p.blocked = false
	p.conns = nil

	p.t.Logf("WebSocketProxy %s revived", p.URL())
}

func (p *WebSocketProxy) acceptLoop() {
	for {
		clientConn, err := p.listener.Accept()
		if err != nil {
			return
		}

		p.mu.Lock()
		blocked := p.blocked
		p.mu.Unlock()
		if blocked {
			_ = clientConn.Close()
			continue
		}

		go p.handleConn(clientConn)
	}
}

func (p *WebSocketProxy) handleConn(clientConn net.Conn) {
	upstream, err := net.Dial("tcp", p.source)
	if err != nil {
		_ = clientConn.Close()
		return
	}

	p.mu.Lock()
	if p.blocked {
		p.mu.Unlock()
		_ = clientConn.Close()
		_ = upstream.Close()
		return
	}
	p.conns = append(p.conns, clientConn, upstream)
	p.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstream, clientConn)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, upstream)
	}()
	wg.Wait()

	p.removeConns(clientConn, upstream)
	_ = clientConn.Close()
	_ = upstream.Close()
}

func (p *WebSocketProxy) removeConns(conns ...net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, target := range conns {
		for i, conn := range p.conns {
			if conn == target {
				p.conns = append(p.conns[:i], p.conns[i+1:]...)
				break
			}
		}
	}
}
