// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cosmos/ibc/cli/internal/store"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
)

// Locks cover planning, checkpoint confirmation and final submission. Entries
// are removed when the last waiter leaves; no global history accumulates.
// Like the existing nonce owner, this coordinates one relayer process.
var clientLocks = struct {
	sync.Mutex
	entries map[[2]string]*clientLock
}{entries: make(map[[2]string]*clientLock)}

type clientLock struct {
	gate  chan struct{}
	users int
}

func lockClient(ctx context.Context, chainID, clientID string) (func(), error) {
	key := [2]string{chainID, clientID}
	clientLocks.Lock()
	entry := clientLocks.entries[key]
	if entry == nil {
		entry = &clientLock{gate: make(chan struct{}, 1)}
		clientLocks.entries[key] = entry
	}
	entry.users++
	clientLocks.Unlock()
	releaseRef := func() {
		clientLocks.Lock()
		defer clientLocks.Unlock()
		entry.users--
		if entry.users == 0 {
			delete(clientLocks.entries, key)
		}
	}
	select {
	case entry.gate <- struct{}{}:
		return func() { <-entry.gate; releaseRef() }, nil
	case <-ctx.Done():
		releaseRef()
		return nil, ctx.Err()
	}
}

func confirmClientUpdate(
	ctx context.Context,
	storage TxStorage,
	submitter txsubmitter.TxSubmitter,
	chainID, clientID string,
	pending store.PacketTx,
) error {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		retry, err := submitter.ShouldRetry(ctx, pending.Hash, pending.Time)
		if errors.Is(err, v2.ErrTxNotFound) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				continue
			}
		}
		if err != nil {
			// Preserve the journal on uncertain outcomes.
			return err
		}
		if err := storage.Transact(ctx, func(repo store.Repository) error {
			return repo.ClearClientUpdate(ctx, chainID, clientID)
		}); err != nil {
			return err
		}
		if retry {
			return fmt.Errorf("client checkpoint %s failed or expired; reprepare from confirmed state", pending.Hash)
		}
		return nil
	}
}
