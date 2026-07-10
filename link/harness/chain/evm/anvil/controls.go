package anvil

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/cosmos/ibc/link/harness/chain"
)

// Chain structurally satisfies BlockController; ProvidesCapability gates it on the mining mode (see below).
var (
	_ chain.BlockController = (*Chain)(nil)
	_ chain.CapabilityGater = (*Chain)(nil)
)

// ProvidesCapability withholds BlockController on an interval-mining node: with --block-time a timer seals
// blocks independently of MineBlocks, so the "pause, submit, observe pending, mine exactly one block"
// contract is unreliable. Instant-mining Anvil (BlockTime == 0) is the only mode that honors it; every other
// capability is offered as-is.
func (ac *Chain) ProvidesCapability(t reflect.Type) bool {
	if t == reflect.TypeOf((*chain.BlockController)(nil)).Elem() {
		return ac.spec.BlockTime == 0
	}
	return true
}

// --- BlockController ---

// MineBlocks mines n blocks in a single anvil_mine call (evm_mine only mines one). n <= 0 is a no-op.
func (ac *Chain) MineBlocks(ctx context.Context, n int) error {
	if n <= 0 {
		return nil
	}
	if err := ac.RPCClient().CallContext(ctx, nil, "anvil_mine", hexutil.Uint64(n)); err != nil {
		return fmt.Errorf("anvil_mine %d: %w", n, err)
	}
	return nil
}

// PauseMining disables automine so submitted transactions remain pending until MineBlocks is called.
func (ac *Chain) PauseMining(ctx context.Context) error {
	if err := ac.RPCClient().CallContext(ctx, nil, "evm_setAutomine", false); err != nil {
		return fmt.Errorf("evm_setAutomine false: %w", err)
	}
	return nil
}

// ResumeMining restores Anvil's normal automine mode.
func (ac *Chain) ResumeMining(ctx context.Context) error {
	if err := ac.RPCClient().CallContext(ctx, nil, "evm_setAutomine", true); err != nil {
		return fmt.Errorf("evm_setAutomine true: %w", err)
	}
	return nil
}

// AdvanceTime fast-forwards the chain clock by d. evm_increaseTime only bumps the offset for the
// next block, so a block is mined to make the new time observable.
func (ac *Chain) AdvanceTime(ctx context.Context, d time.Duration) error {
	// evm_increaseTime has 1-second granularity. Round to nearest second for the normal case, but
	// reject a positive sub-second duration loudly: truncating it to 0 would silently advance time by
	// nothing, which is worse than a clear error.
	if d < 0 {
		return fmt.Errorf("AdvanceTime: duration %v must be non-negative", d)
	}
	secs := uint64(math.Round(d.Seconds()))
	if d > 0 && secs == 0 {
		return fmt.Errorf("AdvanceTime: duration %v is below the 1s granularity of evm_increaseTime", d)
	}
	if err := ac.RPCClient().CallContext(ctx, nil, "evm_increaseTime", secs); err != nil {
		return fmt.Errorf("evm_increaseTime %ds: %w", secs, err)
	}
	if err := ac.MineBlocks(ctx, 1); err != nil {
		return fmt.Errorf("apply time advance: %w", err)
	}
	return nil
}
