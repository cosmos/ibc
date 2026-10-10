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
	"strconv"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

const transactionAbandonmentWindow = 5 * time.Second

// acceptThenFail hides the first successful broadcast response from the relayer.
type acceptThenFail struct {
	server   *httptest.Server
	accepted chan *types.Transaction
	mu       sync.Mutex
	sends    []*types.Transaction
	injected bool
}

func newAcceptThenFail(t *testing.T, endpoint string, before func(context.Context) error) *acceptThenFail {
	t.Helper()
	target, err := url.Parse(endpoint)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	f := &acceptThenFail{accepted: make(chan *types.Transaction, 1)}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
		// Inspect the JSON response consistently even when the node supports gzip.
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
		f.mu.Lock()
		defer f.mu.Unlock()
		var raw hexutil.Bytes
		if len(request.Params) != 1 || json.Unmarshal(request.Params[0], &raw) != nil {
			http.Error(w, "invalid signed transaction", http.StatusBadRequest)
			return
		}
		var tx types.Transaction
		if err = tx.UnmarshalBinary(raw); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.sends = append(f.sends, &tx)
		if !f.injected && before != nil {
			if err = before(r.Context()); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, r)
		var result struct {
			Result string `json:"result"`
		}
		_ = json.Unmarshal(response.Body.Bytes(), &result)
		if !f.injected && result.Result == tx.Hash().Hex() {
			f.injected = true
			f.accepted <- &tx
			http.Error(w, "injected lost broadcast response", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *acceptThenFail) configure(t *testing.T, chainID uint64) func(*ibccli.RelayerConfig) {
	return configureTransactionRPC(t, chainID, f.server.URL)
}

func configureTransactionRPC(t *testing.T, chainID uint64, endpoint string) func(*ibccli.RelayerConfig) {
	return func(cfg *ibccli.RelayerConfig) {
		logPath := filepath.Join(filepath.Dir(cfg.DBPath), "relayer.log")
		t.Cleanup(func() {
			if !t.Failed() {
				return
			}
			data, err := os.ReadFile(logPath)
			if err == nil {
				t.Log(string(data[max(0, len(data)-16000):]))
			}
		})
		for i := range cfg.Chains {
			if cfg.Chains[i].ChainID == strconv.FormatUint(chainID, 10) {
				cfg.Chains[i].RPC = endpoint
				cfg.Chains[i].AbandonAfter = transactionAbandonmentWindow
			}
		}
	}
}

func TestTransactionRecovery_ReceiveAfterLostResponseAndRestart(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(
		e2etest.EVMChains(t, e2etest.EVMRequirements{
			Provider: e2etest.EVMProviderAnvil, ControlledMining: true,
		}, e2etest.ChainA, e2etest.ChainB),
	)
	env := e2etest.Start(t, spec, runtime)
	destination, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	mining, err := destination.Mining()
	require.NoError(t, err)
	fault := newAcceptThenFail(t, destination.RPCURL(), mining.Pause)
	sender, relayerSigner := e2etest.NewSigner(t), e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(
		t,
		env,
		sender,
		relayerSigner,
		fault.configure(t, destination.EVMChainID()),
		route,
	)
	app := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx, cancel := context.WithTimeout(t.Context(), 2*destination.Timing().CompletionBudget)
	defer cancel()
	transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(123456)})
	require.NoError(t, err)
	require.NoError(t, e2etest.RelayAll(ctx, relayer, transfer.PacketTx()))
	var accepted *types.Transaction
	select {
	case accepted = <-fault.accepted:
	case <-ctx.Done():
		t.Fatal("no receive broadcast before deadline")
	}
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, relayer.Stop(ctx))
	client := recoveryClient(t, destination)
	require.NoError(t, client.Client().CallContext(ctx, nil, "anvil_dropAllTransactions"))
	_, _, err = client.TransactionByHash(ctx, accepted.Hash())
	require.ErrorIs(t, err, ethereum.NotFound)
	require.NoError(t, mining.Resume(ctx))
	relayer = e2etest.StartRelayer(t, driver, env)
	deadline, _ := ctx.Deadline()
	var retry *types.Transaction
	require.Eventually(t, func() bool {
		fault.mu.Lock()
		defer fault.mu.Unlock()
		for _, tx := range fault.sends {
			if tx.Hash() != accepted.Hash() {
				_, _, lookupErr := client.TransactionByHash(ctx, tx.Hash())
				if lookupErr == nil {
					retry = tx
					return true
				}
			}
		}
		return false
	}, time.Until(deadline), destination.Timing().PollInterval, "restart must retry the expired transaction")
	require.Equal(t, accepted.Nonce(), retry.Nonce())
	require.Positive(t, retry.GasFeeCap().Cmp(accepted.GasFeeCap()))
	require.Positive(t, retry.GasTipCap().Cmp(accepted.GasTipCap()))
	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(), relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	receipt, err := client.TransactionReceipt(ctx, retry.Hash())
	require.NoError(t, err)
	require.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)
}

func TestTransactionRecovery_AcknowledgementAfterLostResponse(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(t, e2etest.EVMRequirements{}, e2etest.ChainA, e2etest.ChainB))
	env := e2etest.Start(t, spec, runtime)
	source, err := env.Chain(e2etest.ChainA)
	require.NoError(t, err)
	fault := newAcceptThenFail(t, source.RPCURL(), nil)
	sender, relayerSigner := e2etest.NewSigner(t), e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(
		t,
		env,
		sender,
		relayerSigner,
		fault.configure(t, source.EVMChainID()),
		route,
	)
	app := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx, cancel := context.WithTimeout(t.Context(), source.Timing().CompletionBudget)
	defer cancel()
	transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(654321)})
	require.NoError(t, err)
	require.NoError(t, e2etest.RelayAll(ctx, relayer, transfer.PacketTx()))
	_, err = e2etest.AwaitState(ctx, relayer, transfer.PacketTx(), relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
	select {
	case <-fault.accepted:
	default:
		t.Fatal("acknowledgement broadcast fault was not exercised")
	}
}

func TestTransactionRecovery_TimeoutAfterLostResponse(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(
		e2etest.EVMChains(t, e2etest.EVMRequirements{ControlledMining: true}, e2etest.ChainA, e2etest.ChainB),
	)
	env := e2etest.Start(t, spec, runtime)
	source, err := env.Chain(e2etest.ChainA)
	require.NoError(t, err)
	destination, err := env.Chain(e2etest.ChainB)
	require.NoError(t, err)
	fault := newAcceptThenFail(t, source.RPCURL(), nil)
	sender, relayerSigner := e2etest.NewSigner(t), e2etest.NewSigner(t)
	route := e2etest.ManualAtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(
		t,
		env,
		sender,
		relayerSigner,
		fault.configure(t, source.EVMChainID()),
		route,
	)
	app := e2etest.NewTransfer(t, env, deployment, sender, route)
	ctx, cancel := context.WithTimeout(t.Context(), source.Timing().CompletionBudget)
	defer cancel()
	transfer, err := app.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(777), Timeout: packetTimeout})
	require.NoError(t, err)
	mining, err := destination.Mining()
	require.NoError(t, err)
	require.NoError(t, mining.AdvanceTime(ctx, packetTimeoutAdvance))
	relayer := e2etest.StartRelayer(t, driver, env)
	require.NoError(t, e2etest.RelayAll(ctx, relayer, transfer.PacketTx()))
	status, err := e2etest.AwaitState(ctx, relayer, transfer.PacketTx(), relayerv2.PacketState_PACKET_STATE_TIMED_OUT)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyRefunded(ctx, status.GetTimeoutTx().GetTxHash()))
	require.NoError(t, transfer.VerifyNotMinted(ctx))
	require.NoError(t, transfer.VerifyCommitmentCleared(ctx))
	select {
	case accepted := <-fault.accepted:
		require.Equal(
			t,
			accepted.Hash().Hex(),
			status.GetTimeoutTx().GetTxHash(),
			"timeout should resolve original signed attempt",
		)
	default:
		t.Fatal("timeout broadcast fault was not exercised")
	}
}
