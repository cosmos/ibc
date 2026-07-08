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

type shutdownHandler struct {
	stop            chan os.Signal
	mutex           sync.Mutex
	callbacks       []ShutdownFunc
	shutdownTimeout time.Duration
	signalNotify    func(chan<- os.Signal, ...os.Signal)
}

const shutdownTimeout = 10 * time.Second

// Shutdown errors
var (
	ErrForceShutdown   = errors.New("force shutdown occurred")
	ErrTimeoutExceeded = errors.New("shutdown timeout exceeded")
)

var globalHandler *shutdownHandler

func init() {
	setupGlobalHandler()
}

func setupGlobalHandler() {
	notify := make(chan os.Signal, 1)
	signal.Notify(notify, syscall.SIGINT, syscall.SIGTERM)
	globalHandler = newHandler(notify)
}

// AddCallback registers a callback for execution before shutdown.
func AddCallback(fn ShutdownFunc) {
	globalHandler.add(fn)
}

// WaitShutdown waits for application shutdown.
//
// returns ErrForceShutdown if interrupted
// returns ErrTimeoutExceeded if timeout exceeded
func WaitShutdown() error {
	return globalHandler.waitShutdown()
}

func newHandler(notify chan os.Signal) *shutdownHandler {
	return &shutdownHandler{
		stop:            notify,
		shutdownTimeout: shutdownTimeout,
		signalNotify:    signal.Notify,
	}
}

func (h *shutdownHandler) add(fn ShutdownFunc) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.callbacks = append(h.callbacks, fn)
}

func (h *shutdownHandler) callbackSnapshot() []ShutdownFunc {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	callbacks := make([]ShutdownFunc, len(h.callbacks))
	copy(callbacks, h.callbacks)
	return callbacks
}

func (h *shutdownHandler) waitShutdown() error {
	// block until a signal is received
	<-h.stop

	slog.Info("Shutdown signal received")

	// another channel sub to force-quit
	forceNotify := make(chan os.Signal, 1)
	h.signalNotify(forceNotify, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(forceNotify)

	ctx, cancel := context.WithTimeout(context.Background(), h.shutdownTimeout)
	defer cancel()

	callbacks := h.callbackSnapshot()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := len(callbacks) - 1; i >= 0; i-- {
			err := callbacks[i]()
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
