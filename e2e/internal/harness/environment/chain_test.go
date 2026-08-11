// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	chainimpl "github.com/cosmos/ibc/e2e/internal/harness/chain"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
)

func TestFundingUnavailableWithoutManagedControl(t *testing.T) {
	chain := &Chain{id: "attached"}
	chain.bindLease(&environmentLease{})
	funding, err := chain.Funding()
	require.Nil(t, funding)
	require.ErrorIs(t, err, ErrCapabilityUnavailable)
}

func TestEVMTransactionWaitUsesChainTiming(t *testing.T) {
	timing := Timing{
		CompletionBudget: 17 * time.Second,
		PollInterval:     23 * time.Millisecond,
	}
	wait := (&EVM{chain: &Chain{timing: timing}}).transactionWait()
	require.Equal(t, timing.CompletionBudget, wait.Timeout)
	require.Equal(t, timing.PollInterval, wait.PollInterval)
}

func TestProtocolAuthorityFundingSkipsAttachedChains(t *testing.T) {
	authority, err := evm.AccountFromHex(testPrimaryPrivateKeyHex)
	require.NoError(t, err)
	chain := &Chain{id: "attached"}
	chain.bindLease(&environmentLease{})
	require.NoError(t, ensureProtocolAuthorityFunded(t.Context(), chain, authority))
}

func TestProtocolAuthorityFundingUsesManagedCapability(t *testing.T) {
	authority, err := evm.AccountFromHex(testPrimaryPrivateKeyHex)
	require.NoError(t, err)
	controller := &recordingEOAFunder{}
	chain := &Chain{
		id:      "managed",
		funding: &Funding{controller: controller},
	}
	chain.bindLease(&environmentLease{})
	require.NoError(t, ensureProtocolAuthorityFunded(t.Context(), chain, authority))
	require.Equal(t, authority.Address(), controller.address)
	require.Equal(t, "100000000000000000000", controller.minimum.String())
}

type recordingEOAFunder struct {
	address common.Address
	minimum *big.Int
}

func (f *recordingEOAFunder) EnsureEOABalance(_ context.Context, address common.Address, minimum *big.Int) error {
	f.address = address
	f.minimum = new(big.Int).Set(minimum)
	return nil
}

func TestMiningWithPausedUsesFreshContextToResume(t *testing.T) {
	controller := &recordingMining{}
	mining := bindTestMining(controller)
	ctx, cancel := context.WithCancel(t.Context())

	err := mining.WithPaused(ctx, func() error {
		cancel()
		return context.Canceled
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, controller.pauses)
	require.Equal(t, 1, controller.resumes)
	require.NoError(t, controller.resumeContextError, "resume must not inherit callback cancellation")
}

type recordingMining struct {
	pauses             int
	resumes            int
	resumeContextError error
}

func (m *recordingMining) PauseMining(context.Context) error {
	m.pauses++
	return nil
}

func (m *recordingMining) ResumeMining(ctx context.Context) error {
	m.resumes++
	m.resumeContextError = ctx.Err()
	return nil
}

func (*recordingMining) MineBlocks(context.Context, int) error            { return nil }
func (*recordingMining) AdvanceTime(context.Context, time.Duration) error { return nil }

func TestMiningWithPausedJoinsCallbackAndResumeFailures(t *testing.T) {
	callbackErr := errors.New("callback")
	resumeErr := errors.New("resume")
	controller := &failingResumeMining{resumeErr: resumeErr}
	err := bindTestMining(controller).WithPaused(
		t.Context(),
		func() error { return callbackErr },
	)
	require.ErrorIs(t, err, callbackErr)
	require.ErrorIs(t, err, resumeErr)
}

type failingResumeMining struct{ resumeErr error }

func (*failingResumeMining) PauseMining(context.Context) error                { return nil }
func (m *failingResumeMining) ResumeMining(context.Context) error             { return m.resumeErr }
func (*failingResumeMining) MineBlocks(context.Context, int) error            { return nil }
func (*failingResumeMining) AdvanceTime(context.Context, time.Duration) error { return nil }

func bindTestMining(controller chainimpl.BlockController) *Mining {
	mining := &Mining{controller: controller}
	chain := &Chain{id: "test", mining: mining}
	chain.bindLease(&environmentLease{})
	return mining
}
