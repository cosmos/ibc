package testapp

import (
	"context"
	"fmt"
	"time"
)

// await retries transient observation errors until the probe succeeds or the
// Chain's completion budget expires. A probe reports done with an error when
// the observed result itself is invalid and retrying cannot fix it.
func await[T any](
	ctx context.Context,
	budget, poll time.Duration,
	description string,
	probe func(context.Context) (T, bool, error),
) (T, error) {
	var zero T
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	var lastErr error
	for {
		value, done, err := probe(ctx)
		switch {
		case done && err != nil:
			return zero, err
		case done:
			return value, nil
		case err != nil:
			lastErr = err
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return zero, fmt.Errorf(
					"waiting for %s: %w (last observation error: %w)",
					description,
					ctx.Err(),
					lastErr,
				)
			}
			return zero, fmt.Errorf("waiting for %s: %w", description, ctx.Err())
		case <-ticker.C:
		}
	}
}
