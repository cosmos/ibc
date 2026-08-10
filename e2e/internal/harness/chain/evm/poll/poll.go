// SPDX-License-Identifier: Apache-2.0

// Package poll provides readiness polling for EVM clients and launchers.
package poll

import (
	"context"
	"time"
)

// Until returns the first predicate error and does not retry it.
func Until(
	ctx context.Context,
	interval time.Duration,
	pred func(context.Context) (done bool, err error),
) error {
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
