package onchain

import (
	"context"
	"fmt"
	"time"
)

// Await is the single effect-wait primitive the per-family Readers (and the status cross-check) share:
// a bounded re-probe of chain or status state at the observed chain's cadence, per the repo-wide
// "never sleep; poll semantic conditions" rule. It bounds the wait by budget, probes immediately and
// then at every poll tick, and reports a deadline as "waiting for <desc>", carrying the last retained
// probe error so a timeout names what the wait was stuck on.
//
// The probe's return contract encodes the transient-vs-fatal distinction every effect wait needs:
//
//   - (v, true, nil): the awaited condition holds; v is returned.
//   - (_, true, err): a fatal observation (e.g. the matched effect contradicts the action);
//     err is returned as-is, immediately.
//   - (_, false, err): the probe could not observe (transient RPC hiccup) or observed
//     not-yet progress; err is retained for the deadline message and the wait continues.
//   - (_, false, nil): nothing observed yet; keep polling.
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
