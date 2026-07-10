// Package poll holds the single readiness-probe primitive shared by the launch-side packages (EVM
// client/providers, docker helpers) — condition polling, never a fixed sleep. It sits below the chain
// packages and imports nothing, so anything on the launch path can use it without pulling in the reader
// layer; effect waits above the launch path use onchain.Await instead (which retries through transient
// probe errors, where Until aborts on the first one).
package poll

import (
	"context"
	"time"
)

// Until invokes pred every interval until it reports done, ctx is canceled, or timeout elapses
// (whichever comes first). A pred error aborts immediately and is returned as-is; a deadline or
// cancellation is surfaced as ctx.Err() so callers can wrap it with their own context.
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
