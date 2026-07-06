// Package graceful contains API for working with graceful application shutdown.
//
// Application starts listening for SIGINT or SIGTERM signals and handles them properly.
package graceful

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// ShutdownFunc is a callback-type for registering callbacks before application shutdown.
type ShutdownFunc func() error

const shutdownTimeout = 10 * time.Second

// Shutdown errors
var (
	ErrForceShutdown   = errors.New("force shutdown occurred")
	ErrTimeoutExceeded = errors.New("shutdown timeout exceeded")
)

var handler *shutdownHandler

func init() {
	setupHandler()
}

func setupHandler() {
	notify := make(chan os.Signal, 1)
	signal.Notify(notify, syscall.SIGINT, syscall.SIGTERM)
	handler = newHandler(notify)
}

// AddCallback registers a callback for execution before shutdown.
func AddCallback(fn ShutdownFunc) {
	handler.add(fn)
}

// WaitShutdown waits for application shutdown.
//
// returns ErrForceShutdown if interrupted
// returns ErrTimeoutExceeded if timeout exceeded
func WaitShutdown() error {
	// block until a signal is received
	<-handler.stop

	slog.Info("Shutdown signal received")

	// another channel sub to force-quite
	forceNotify := make(chan os.Signal, 1)
	signal.Notify(forceNotify, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := len(handler.callbacks) - 1; i >= 0; i-- {
			err := handler.callbacks[i]()
			if err != nil {
				slog.Error("Shutdown callback error", "err", err)
			}
		}
	}()

	select {
	case <-done:
		return nil
	case <-forceNotify:
		return ErrForceShutdown
	case <-ctx.Done():
		return ErrTimeoutExceeded
	}
}

type shutdownHandler struct {
	stop      chan os.Signal
	mutex     sync.Mutex
	callbacks []ShutdownFunc
}

func newHandler(notify chan os.Signal) *shutdownHandler {
	return &shutdownHandler{
		stop: notify,
	}
}

func (h *shutdownHandler) add(fn ShutdownFunc) {
	h.mutex.Lock()
	h.callbacks = append(h.callbacks, fn)
	h.mutex.Unlock()
}
