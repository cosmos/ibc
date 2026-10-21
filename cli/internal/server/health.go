// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
)

const readinessTimeout = time.Second

type healthChecker struct {
	// Handlers are registered before the server starts and remain immutable.
	handlers map[string]Handler
	mu       sync.RWMutex
	serving  bool
	stopped  bool
}

func (h *healthChecker) start() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped {
		return http.ErrServerClosed
	}
	h.serving = true
	return nil
}

func (h *healthChecker) stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.serving = false
	h.stopped = true
}

func (h *healthChecker) isServing() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.serving
}

// Check evaluates readiness on demand; querying the health service itself only checks liveness.
func (h *healthChecker) Check(ctx context.Context, req *grpchealth.CheckRequest) (*grpchealth.CheckResponse, error) {
	handler, registered := h.handlers[req.Service]
	if req.Service != "" && req.Service != grpchealth.HealthV1ServiceName && !registered {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("unknown service %q", req.Service))
	}
	res := &grpchealth.CheckResponse{Status: grpchealth.StatusNotServing}
	if !h.isServing() {
		return res, nil
	}

	ctx, cancel := context.WithTimeout(ctx, readinessTimeout)
	defer cancel()
	switch req.Service {
	case grpchealth.HealthV1ServiceName:
	case "":
		for _, handler := range h.handlers {
			if err := handler.CheckReady(ctx); err != nil {
				return res, nil
			}
		}
	default:
		if err := handler.CheckReady(ctx); err != nil {
			return res, nil
		}
	}
	// A dependency returning late must not restore readiness during shutdown.
	if ctx.Err() == nil && h.isServing() {
		res.Status = grpchealth.StatusServing
	}
	return res, nil
}
