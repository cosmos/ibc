package onchain

import (
	"context"
	"fmt"
	"time"
)

// Await polls until the probe reports done or the budget expires. Errors with done=false are
// retained for the deadline; errors with done=true are returned immediately.
func Await[T any](
	ctx context.Context,
	budget, poll time.Duration,
	desc string,
	probe func(context.Context) (T, bool, error),
) (T, error) {
	var zero T
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	var lastErr error
	for {
		v, done, err := probe(ctx)
		switch {
		case done && err != nil:
			return zero, err
		case done:
			return v, nil
		case err != nil:
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return zero, fmt.Errorf("waiting for %s: %w (last probe error: %w)", desc, ctx.Err(), lastErr)
			}
			return zero, fmt.Errorf("waiting for %s: %w", desc, ctx.Err())
		case <-ticker.C:
		}
	}
}
