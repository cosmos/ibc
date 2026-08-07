package anvil

import (
	"context"
	"errors"
	"io"
	"math/big"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
)

func TestMixedMiningPauseResume(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("Docker is unavailable: %v", err)
	}
	if output, err := exec.CommandContext(t.Context(), "docker", "info").CombinedOutput(); err != nil {
		t.Skipf("Docker daemon is unavailable: %v (%s)", err, output)
	}

	chain, err := Start(t.Context(), Spec{
		ID: "mixed-mining", ChainID: 31367, LogPath: filepath.Join(t.TempDir(), "anvil.log"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, chain.Stop()) })
	inspection, err := chain.container.Inspect(t.Context())
	require.NoError(t, err)
	require.Contains(t, inspection.Args, "--mixed-mining")

	account, err := evm.NewAccount()
	require.NoError(t, err)
	require.NoError(t, chain.EnsureEOABalance(t.Context(), account.Address(), big.NewInt(1_000_000_000_000_000_000)))
	recipient := common.HexToAddress("0x1000000000000000000000000000000000000001")
	wait := evm.TransactionWait{Timeout: 4 * time.Second, PollInterval: 50 * time.Millisecond}
	// Observe idle progress separately from transaction-triggered mining.
	beforeIdle, err := chain.Height(t.Context())
	require.NoError(t, err)
	requireHeightAdvance(t, chain, beforeIdle)

	_, err = broadcast(t.Context(), chain, wait, account, recipient)
	require.NoError(t, err, "mixed mining must include an ordinary transaction")
	afterTransaction, err := chain.Height(t.Context())
	require.NoError(t, err)
	requireHeightAdvance(t, chain, afterTransaction)

	require.NoError(t, chain.PauseMining(t.Context()))
	pausedHeight, err := chain.Height(t.Context())
	require.NoError(t, err)

	result := make(chan broadcastResult, 1)
	go func() {
		receipt, broadcastErr := broadcast(t.Context(), chain, wait, account, recipient)
		result <- broadcastResult{receipt: receipt, err: broadcastErr}
	}()
	requirePendingTx(t, chain, wait)
	time.Sleep(1200 * time.Millisecond)
	stillPaused, err := chain.Height(t.Context())
	require.NoError(t, err)
	require.Equal(t, pausedHeight, stillPaused)
	select {
	case got := <-result:
		t.Fatalf("transaction completed while mining was paused: receipt=%v err=%v", got.receipt, got.err)
	default:
	}

	require.NoError(t, chain.ResumeMining(t.Context()))
	select {
	case got := <-result:
		require.NoError(t, got.err)
		require.NotNil(t, got.receipt)
	case <-time.After(3 * time.Second):
		t.Fatal("transaction was not mined after resume")
	}
	resumedHeight, err := chain.Height(t.Context())
	require.NoError(t, err)
	requireHeightAdvance(t, chain, resumedHeight)
}

type broadcastResult struct {
	receipt *types.Receipt
	err     error
}

func broadcast(
	ctx context.Context,
	chain *Chain,
	wait evm.TransactionWait,
	account evm.Account,
	recipient common.Address,
) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := chain.WithEVMClient(func(client *evm.EVMClient) error {
		var err error
		receipt, err = client.BroadcastTx(ctx, wait, account, &recipient, nil, big.NewInt(1))
		return err
	})
	return receipt, err
}

func requirePendingTx(t *testing.T, chain *Chain, wait evm.TransactionWait) {
	t.Helper()
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		var pending uint
		err := chain.WithEVMClient(func(client *evm.EVMClient) error {
			var err error
			pending, err = client.Client().PendingTransactionCount(t.Context())
			return err
		})
		require.NoError(collect, err)
		require.NotZero(collect, pending)
	}, wait.Timeout, wait.PollInterval)
}

func requireHeightAdvance(t *testing.T, chain *Chain, from uint64) {
	t.Helper()
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		height, err := chain.Height(t.Context())
		require.NoError(collect, err)
		require.Greater(collect, height, from)
	}, 3*time.Second, 50*time.Millisecond)
}

func TestStopSucceedsWhenRemovalSucceedsAfterGracefulStopFailure(t *testing.T) {
	container := &stopTestContainer{stopErr: errors.New("graceful stop failed")}
	chain := &Chain{container: container}

	require.NoError(t, chain.Stop())
	require.True(t, chain.closed)
	require.True(t, chain.stopped)
	require.Nil(t, chain.container)
	require.Equal(t, 1, container.terminateCalls)
}

type stopTestContainer struct {
	testcontainers.Container
	stopErr        error
	terminateCalls int
}

func (*stopTestContainer) IsRunning() bool { return true }

func (c *stopTestContainer) Stop(context.Context, *time.Duration) error { return c.stopErr }

func (c *stopTestContainer) Terminate(context.Context, ...testcontainers.TerminateOption) error {
	c.terminateCalls++
	return nil
}

func (*stopTestContainer) Logs(context.Context) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
