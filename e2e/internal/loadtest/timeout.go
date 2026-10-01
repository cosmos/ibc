package loadtest

import (
	"testing"
	"time"
)

const (
	// deadlineMargin is left before the go test -timeout deadline so cleanup can run.
	// timeoutFallback covers -timeout 0.
	deadlineMargin  = 1 * time.Minute
	timeoutFallback = 10 * time.Minute
)

// TestTimeout is the time left on the go test -timeout deadline, minus deadlineMargin.
// The same context covers submit and the post-submit await.
func TestTimeout(t *testing.T) time.Duration {
	t.Helper()

	deadline, ok := t.Deadline()
	if !ok {
		return timeoutFallback
	}

	remaining := time.Until(deadline) - deadlineMargin
	if remaining <= 0 {
		t.Fatalf("e2e deadline is within %s", deadlineMargin)
	}

	return remaining.Truncate(time.Second)
}
