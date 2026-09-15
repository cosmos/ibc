// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cosmos/ibc/cli/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// clientLocks serializes relay work per light client within this process, so
// a checkpoint is confirmed before another batch prepares against it. Keys are
// bounded by the configured connections and never removed.
var clientLocks sync.Map // [2]string{chainID, clientID} -> chan struct{}

func lockClient(ctx context.Context, chainID, clientID string) (func(), error) {
	gate, _ := clientLocks.LoadOrStore([2]string{chainID, clientID}, make(chan struct{}, 1))
	ch := gate.(chan struct{})
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// confirmClientUpdate waits until the checkpoint transaction lands and fails
// if it reverted or expired, so the next Prepare reads confirmed state.
func confirmClientUpdate(ctx context.Context, submitter txsubmitter.TxSubmitter, sub v2.Submission) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		retry, err := submitter.ShouldRetry(ctx, sub.TxHash, sub.SubmittedAt)
		switch {
		case errors.Is(err, v2.ErrTxNotFound):
		case err != nil:
			return err
		case retry:
			return fmt.Errorf("client checkpoint %s failed or expired", sub.TxHash)
		default:
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
