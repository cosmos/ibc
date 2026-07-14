package anvil

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/chain/evm"
)

var _ chain.BlockController = (*Chain)(nil)

func (ac *Chain) MineBlocks(ctx context.Context, n int) error {
	if n <= 0 {
		return nil
	}
	return ac.WithEVMClient(func(client *evm.EVMClient) error {
		if err := client.RPCClient().CallContext(ctx, nil, "anvil_mine", hexutil.Uint64(n)); err != nil {
			return fmt.Errorf("anvil_mine %d: %w", n, err)
		}
		return nil
	})
}

func (ac *Chain) PauseMining(ctx context.Context) error {
	return ac.WithEVMClient(func(client *evm.EVMClient) error {
		if err := client.RPCClient().CallContext(ctx, nil, "evm_setAutomine", false); err != nil {
			return fmt.Errorf("evm_setAutomine false: %w", err)
		}
		return nil
	})
}

func (ac *Chain) ResumeMining(ctx context.Context) error {
	return ac.WithEVMClient(func(client *evm.EVMClient) error {
		if err := client.RPCClient().CallContext(ctx, nil, "evm_setAutomine", true); err != nil {
			return fmt.Errorf("evm_setAutomine true: %w", err)
		}
		return nil
	})
}

func (ac *Chain) AdvanceTime(ctx context.Context, d time.Duration) error {
	// evm_increaseTime has whole-second granularity; positive durations rounding to zero are rejected.
	if d < 0 {
		return fmt.Errorf("AdvanceTime: duration %v must be non-negative", d)
	}
	secs := uint64(math.Round(d.Seconds()))
	if d > 0 && secs == 0 {
		return fmt.Errorf("AdvanceTime: duration %v is below the 1s granularity of evm_increaseTime", d)
	}
	return ac.WithEVMClient(func(client *evm.EVMClient) error {
		if err := client.RPCClient().CallContext(ctx, nil, "evm_increaseTime", secs); err != nil {
			return fmt.Errorf("evm_increaseTime %ds: %w", secs, err)
		}
		if err := client.RPCClient().CallContext(ctx, nil, "anvil_mine", hexutil.Uint64(1)); err != nil {
			return fmt.Errorf("apply time advance: anvil_mine 1: %w", err)
		}
		return nil
	})
}
