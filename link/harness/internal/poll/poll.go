// Package poll provides readiness polling for chain launchers.
package poll

import (
	"context"
	"time"
)

// Until returns the first predicate error and does not retry it.
func Until(
	ctx context.Context,
	interval, timeout time.Duration,
	pred func(context.Context) (done bool, err error),
) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		done, err := pred(ctx)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
