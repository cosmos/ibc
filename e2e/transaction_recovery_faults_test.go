// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
	"github.com/cosmos/ibc/gen/go/solidity-abi/accessmanager"
)

// broadcastProxy injects a fault before the first broadcast reaches the node
// and reports only transactions the node actually accepts.
type broadcastProxy struct {
	server   *httptest.Server
	accepted chan *types.Transaction
}

func newBroadcastProxy(
	t *testing.T,
	endpoint string,
	before func(context.Context, *types.Transaction) error,
) *broadcastProxy {
	t.Helper()
	target, err := url.Parse(endpoint)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	f := &broadcastProxy{accepted: make(chan *types.Transaction, 128)}
	var mu sync.Mutex
	first := true
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.Header.Set("Accept-Encoding", "identity")
		var request struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(body, &request)
		if request.Method != "eth_sendRawTransaction" {
			proxy.ServeHTTP(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		var raw hexutil.Bytes
		var tx types.Transaction
		if len(request.Params) != 1 || json.Unmarshal(request.Params[0], &raw) != nil ||
			tx.UnmarshalBinary(raw) != nil {
			http.Error(w, "invalid signed transaction", http.StatusBadRequest)
			return
		}
		if first {
			first = false
			if before != nil {
				if err := before(r.Context(), &tx); err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
			}
		}
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, r)
		var result struct {
			Result string `json:"result"`
		}
		_ = json.Unmarshal(response.Body.Bytes(), &result)
		if result.Result == tx.Hash().Hex() {
			select {
			case f.accepted <- &tx:
			case <-r.Context().Done():
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *broadcastProxy) awaitAccepted(
	ctx context.Context,
	t *testing.T,
	matches func(*types.Transaction) bool,
) *types.Transaction {
	t.Helper()
	for {
		select {
		case tx := <-f.accepted:
			if matches == nil || matches(tx) {
				return tx
			}
		case <-ctx.Done():
			t.Fatal("no matching transaction accepted before deadline:", ctx.Err())
		}
	}
}

func recoveryClient(t *testing.T, chain *environment.Chain) *ethclient.Client {
	t.Helper()
	client, err := ethclient.Dial(chain.RPCURL())
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client
}

func awaitTransactionResolution(
	ctx context.Context,
	t *testing.T,
	chain *environment.Chain,
	logPath string,
	tx *types.Transaction,
	status string,
) {
	t.Helper()
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(logPath)
		if err != nil {
			return false
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			var entry struct {
				Message string `json:"msg"`
				Hash    string `json:"hash"`
				Status  string `json:"status"`
			}
			if json.Unmarshal([]byte(line), &entry) == nil && entry.Message == "Transaction nonce resolved" &&
				entry.Hash == tx.Hash().Hex() && entry.Status == status {
				return true
			}
		}
		return false
	}, time.Until(deadline), chain.Timing().PollInterval, "transaction must be resolved before injecting the next fault")
}

func TestTransactionRecovery_RepricesPendingReceiveAfterRestart(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(
		t,
		e2etest.EVMRequirements{
			Provider:         e2etest.EVMProviderAnvil,
			ControlledMining: true,
		},
		e2etest.ChainA,
		e2etest.ChainB,
	))
	env := e2etest.Start(t, spec, runtime)
	destination, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := destination.Mining()
	require.NoError(t, err)
	fault := newBroadcastProxy(t, destination.RPCURL(), func(ctx context.Context, _ *types.Transaction) error {
		return mining.Pause(ctx)
	})
	sender, relayerSigner := e2etest.NewSigner(t), e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		configureTransactionRPC(t, destination.EVMChainID(), fault.server.URL), route)
	app := e2etest.NewTransfer(t, env, deployment, sender, route)
	// Keep the paused chain ahead of the relayer's freshness check until replacement.
	require.NoError(t, mining.AdvanceTime(t.Context(), 2*destination.Timing().CompletionBudget))
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx, cancel := context.WithTimeout(t.Context(), 2*destination.Timing().CompletionBudget)
	defer cancel()
	transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(123456)})
	require.NoError(t, err)
	require.NoError(t, e2etest.RelayAll(ctx, relayer, transfer.PacketTx()))
	original := fault.awaitAccepted(ctx, t, nil)
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, relayer.Crash(ctx))
	client := recoveryClient(t, destination)
	_, pending, err := client.TransactionByHash(ctx, original.Hash())
	require.NoError(t, err)
	require.True(t, pending, "original transaction must remain in the mempool")
	_, err = client.TransactionReceipt(ctx, original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	relayer = e2etest.StartRelayer(t, driver, env)

	replacement := fault.awaitAccepted(ctx, t, func(tx *types.Transaction) bool { return tx.Hash() != original.Hash() })
	require.Equal(t, original.Nonce(), replacement.Nonce())
	require.Equal(t, original.To(), replacement.To())
	require.Equal(t, original.Value(), replacement.Value())
	require.Positive(t, replacement.GasFeeCap().Cmp(original.GasFeeCap()))
	require.Positive(t, replacement.GasTipCap().Cmp(original.GasTipCap()))
	require.NoError(t, mining.Mine(ctx, 1))
	receipt, err := client.TransactionReceipt(ctx, replacement.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
	require.NoError(t, mining.Resume(ctx))
	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(), relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.Equal(t, replacement.Hash().Hex(), status.GetRecvTx().GetTxHash())
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
	_, err = client.TransactionReceipt(ctx, original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
}

func TestTransactionRecovery_CrashBeforeBroadcast(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	destination, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	intercepted := make(chan *types.Transaction, 1)
	fault := newBroadcastProxy(t, destination.RPCURL(), func(ctx context.Context, tx *types.Transaction) error {
		intercepted <- tx
		<-ctx.Done()
		return ctx.Err()
	})
	sender, relayerSigner := e2etest.NewSigner(t), e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(t, env, sender, relayerSigner,
		func(cfg *ibccli.RelayerConfig) {
			configureTransactionRPC(t, destination.EVMChainID(), fault.server.URL)(cfg)
			withBatchOverride(cfg)
		}, route)
	app := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx, cancel := context.WithTimeout(t.Context(), 2*destination.Timing().CompletionBudget)
	defer cancel()
	batch, err := app.SendBatch(ctx, []e2etest.IFTRequest{
		{Amount: big.NewInt(234567)},
		{Amount: big.NewInt(345678)},
		{Amount: big.NewInt(456789)},
	})
	require.NoError(t, err)
	packets := batch.PacketTxs()
	require.Len(t, packets, 3)
	require.NoError(t, e2etest.RelayAll(ctx, relayer, packets[0]))
	var saved *types.Transaction
	select {
	case saved = <-intercepted:
	case <-ctx.Done():
		t.Fatal("no signed transaction reached the broadcast barrier")
	}
	require.NoError(t, relayer.Crash(ctx))
	client := recoveryClient(t, destination)
	_, _, err = client.TransactionByHash(ctx, saved.Hash())
	require.ErrorIs(t, err, ethereum.NotFound, "the node must never have seen the saved transaction")
	relayer = e2etest.StartRelayer(t, driver, env)
	broadcast := fault.awaitAccepted(ctx, t, nil)
	require.Equal(t, saved.Nonce(), broadcast.Nonce())
	require.Positive(t, broadcast.GasFeeCap().Cmp(saved.GasFeeCap()))
	require.Positive(t, broadcast.GasTipCap().Cmp(saved.GasTipCap()))
	for _, packet := range packets {
		status, statusErr := e2etest.AwaitState(ctx, relayer, packet, relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
		require.NoError(t, statusErr)
		require.Equal(
			t,
			broadcast.Hash().Hex(),
			status.GetRecvTx().GetTxHash(),
			"all packets must recover their shared transaction",
		)
	}
	require.NoError(t, batch.VerifyDelivered(ctx))
	require.NoError(t, batch.VerifyBurned(ctx), "successful acknowledgements must not refund")
}

func TestTransactionRecovery_ReorgAfterResolution(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(
		t,
		e2etest.EVMRequirements{
			Provider:         e2etest.EVMProviderAnvil,
			ControlledMining: true,
		},
		e2etest.ChainA,
		e2etest.ChainB,
	))
	env := e2etest.Start(t, spec, runtime)
	destination, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := destination.Mining()
	require.NoError(t, err)
	client := recoveryClient(t, destination)
	var snapshot string
	fault := newBroadcastProxy(t, destination.RPCURL(), func(ctx context.Context, _ *types.Transaction) error {
		if pauseErr := mining.Pause(ctx); pauseErr != nil {
			return pauseErr
		}
		return client.Client().CallContext(ctx, &snapshot, "evm_snapshot")
	})
	sender, relayerSigner := e2etest.NewSigner(t), e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	var logPath string
	driver, deployment := e2etest.DeployWithRelayerConfig(
		t,
		env,
		sender,
		relayerSigner,
		func(cfg *ibccli.RelayerConfig) {
			configureTransactionRPC(t, destination.EVMChainID(), fault.server.URL)(cfg)
			logPath = filepath.Join(filepath.Dir(cfg.DBPath), "relayer.log")
		},
		route,
	)
	app := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx, cancel := context.WithTimeout(t.Context(), 2*destination.Timing().CompletionBudget)
	defer cancel()
	transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(345678)})
	require.NoError(t, err)
	require.NoError(t, e2etest.RelayAll(ctx, relayer, transfer.PacketTx()))
	original := fault.awaitAccepted(ctx, t, nil)
	require.NoError(t, mining.Mine(ctx, 1))
	receipt, err := client.TransactionReceipt(ctx, original.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
	awaitTransactionResolution(ctx, t, destination, logPath, original, "CONFIRMED")
	require.NoError(t, transfer.VerifyCommitmentPresent(ctx))
	require.NoError(t, relayer.Crash(ctx))

	var reverted bool
	require.NoError(t, client.Client().CallContext(ctx, &reverted, "evm_revert", snapshot))
	require.True(t, reverted)
	require.NoError(t, client.Client().CallContext(ctx, nil, "anvil_dropAllTransactions"))
	// Restore the old height on a different fork without mining the original transaction.
	require.NoError(t, mining.AdvanceTime(ctx, time.Second))
	_, err = client.TransactionReceipt(ctx, original.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	require.NoError(t, transfer.VerifyReceiptAbsent(ctx))
	// Discard broadcasts observed before the crash.
	for len(fault.accepted) > 0 {
		<-fault.accepted
	}
	require.NoError(t, mining.Resume(ctx))
	relayer = e2etest.StartRelayer(t, driver, env)
	rebroadcast := fault.awaitAccepted(ctx, t, nil)
	require.Equal(t, original.Nonce(), rebroadcast.Nonce())
	require.Positive(t, rebroadcast.GasFeeCap().Cmp(original.GasFeeCap()))
	require.Positive(t, rebroadcast.GasTipCap().Cmp(original.GasTipCap()))
	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(), relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
	canonical, err := client.TransactionReceipt(ctx, rebroadcast.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, canonical.Status)
	require.NotEqual(t, receipt.BlockHash, canonical.BlockHash)
}

func TestTransactionRecovery_RetriesMinedFailure(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	destination, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	instance, err := env.IBCInstanceForChain(e2etest.ChainB)
	require.NoError(t, err)
	evmAccess, err := destination.EVM()
	require.NoError(t, err)
	authority, err := evm.AccountFromHex(runtime.Authorities[e2etest.ProtocolAuthorityID].PrivateKeyHex)
	require.NoError(t, err)
	manager := common.HexToAddress(string(instance.AccessManagerAddress()))
	router := common.HexToAddress(string(instance.Locator()))
	abi, err := accessmanager.AccessManagerMetaData.GetAbi()
	require.NoError(t, err)
	setClosed := func(ctx context.Context, closed bool) error {
		data, packErr := abi.Pack("setTargetClosed", router, closed)
		if packErr != nil {
			return packErr
		}
		_, sendErr := evmAccess.BroadcastTx(ctx, authority, &manager, data, nil)
		return sendErr
	}
	// Close the router after the relayer has simulated and signed its receive.
	fault := newBroadcastProxy(t, destination.RPCURL(), func(ctx context.Context, _ *types.Transaction) error {
		return setClosed(ctx, true)
	})
	sender, relayerSigner := e2etest.NewSigner(t), e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	var logPath string
	driver, deployment := e2etest.DeployWithRelayerConfig(
		t,
		env,
		sender,
		relayerSigner,
		func(cfg *ibccli.RelayerConfig) {
			configureTransactionRPC(t, destination.EVMChainID(), fault.server.URL)(cfg)
			logPath = filepath.Join(filepath.Dir(cfg.DBPath), "relayer.log")
		},
		route,
	)
	app := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx, cancel := context.WithTimeout(t.Context(), 2*destination.Timing().CompletionBudget)
	defer cancel()
	transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(456789)})
	require.NoError(t, err)
	require.NoError(t, e2etest.RelayAll(ctx, relayer, transfer.PacketTx()))
	failed := fault.awaitAccepted(ctx, t, nil)
	receipt, err := evmAccess.AwaitTransactionReceipt(ctx, failed.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusFailed, receipt.Status)
	require.NoError(t, transfer.VerifyReceiptAbsent(ctx))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	awaitTransactionResolution(ctx, t, destination, logPath, failed, "FAILED")
	require.NoError(t, setClosed(ctx, false))

	retry := fault.awaitAccepted(ctx, t, func(tx *types.Transaction) bool { return tx.Nonce() > failed.Nonce() })
	require.Equal(t, failed.Nonce()+1, retry.Nonce())
	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(), relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.Equal(t, retry.Hash().Hex(), status.GetRecvTx().GetTxHash())
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
}
