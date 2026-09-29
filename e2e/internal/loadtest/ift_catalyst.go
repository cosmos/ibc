package loadtest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethrunner "github.com/skip-mev/catalyst/chains/ethereum/runner"
	catalystevm "github.com/skip-mev/catalyst/chains/ethereum/types"
	catalyst "github.com/skip-mev/catalyst/chains/types"
	"github.com/skip-mev/catalyst/ift/accounts"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

type SpecIFT struct {
	numWallets      int
	totalPackets    int
	packetsPerBlock int
	catalyst        catalyst.LoadTestSpec
	wallets         []common.Address
}

type EVMEndpoint struct {
	ChainID   string
	RPC       string
	Websocket string
}

type IFTToken struct {
	ClientID string
	Address  string
}

type relayerAPI interface {
	PacketStatuses(ctx context.Context, sourceChainID, sourceTxHash string) ([]*relayerv2.PacketStatus, error)
}

const (
	msgTypeIFTTransfer = "MsgIFTTransfer"
	packetTimeout      = time.Hour

	// wei value
	defaultTokenTransferAmount = "10000"

	catalystPacketPoll = 6 * time.Second
	catalystTxLookups  = 10
)

func NewSpecIFT(
	wallets int,
	totalPackets int,
	packetsPerBlock int,
	baseMnemonic string,
	chain EVMEndpoint,
	token IFTToken,
) (*SpecIFT, error) {
	switch {
	case wallets <= 0:
		return nil, fmt.Errorf("wallets must be positive")
	case packetsPerBlock <= 0:
		return nil, fmt.Errorf("packets per block must be positive")
	case totalPackets <= 0 || totalPackets%packetsPerBlock != 0:
		return nil, fmt.Errorf(
			"total packets %d is not divisible by packets per block %d",
			totalPackets,
			packetsPerBlock,
		)
	}

	spec := &SpecIFT{
		numWallets:      wallets,
		totalPackets:    totalPackets,
		packetsPerBlock: packetsPerBlock,
	}

	spec.catalyst = catalyst.LoadTestSpec{
		Kind:        catalyst.KindEVM,
		Name:        "IFT",
		Description: "IFT auto-relay load test",

		// Block scans count every receipt in the window, including relayer txs.
		SkipReceiptCollection: true,

		NumOfBlocks: totalPackets / packetsPerBlock,

		NumWallets:     wallets,
		InitialWallets: wallets,
		BaseMnemonic:   baseMnemonic,

		ChainID: chain.ChainID,
		ChainCfg: &catalystevm.ChainConfig{
			NodesAddresses: []catalystevm.NodeAddress{{
				RPC:       chain.RPC,
				Websocket: chain.Websocket,
			}},
		},

		IFT: &catalyst.IFTConfig{
			ClientID: token.ClientID,
			Amount:   defaultTokenTransferAmount,
			Timeout:  packetTimeout,
			Recipients: catalyst.IFTRecipientsConfig{
				// zero == use ALL wallets
				Count: 0,
				// use recipients starting from the second wallet
				// zero means range from=len(wallets) to=len(wallets)*2 which is strange
				// because recipients won't be able to send back in case of bi-directional IFT
				Offset: 1,
			},
			Destination: catalyst.IFTDestinationConfig{
				Kind: catalyst.KindEVM,
				EVM:  &catalyst.IFTDestinationEVMConfig{},
			},
			EVM: &catalyst.IFTEVMConfig{
				ContractAddress: token.Address,
			},
		},

		Msgs: []catalyst.LoadTestMsg{{
			Type:    msgTypeIFTTransfer,
			NumMsgs: packetsPerBlock,
		}},
	}

	if err := spec.catalyst.Validate(); err != nil {
		return nil, fmt.Errorf("catalyst: %w", err)
	}

	return spec, nil
}

// Wallets returns addresses 0..wallets-1 derived from the spec mnemonic.
// The result is cached. Catalyst derives the same keys when it runs.
func (s *SpecIFT) Wallets() ([]common.Address, error) {
	if len(s.wallets) > 0 {
		return s.wallets, nil
	}

	wallets := make([]common.Address, s.numWallets)
	for i := range s.numWallets {
		addr, err := accounts.EVMAddressFromMnemonic(s.catalyst.BaseMnemonic, i)
		if err != nil {
			return nil, fmt.Errorf("wallet %d: %w", i, err)
		}
		wallets[i] = addr
	}

	s.wallets = wallets

	return wallets, nil
}

// Run executes this spec in-process.
func (s *SpecIFT) Run(ctx context.Context, t testing.TB) (catalyst.LoadTestResult, []*catalystevm.SentTx, error) {
	runner, err := ethrunner.NewRunner(ctx, zap.NewNop(), s.catalyst)
	require.NoError(t, err)

	result, err := runner.Run(ctx)
	if err != nil {
		return catalyst.LoadTestResult{}, nil, err
	}

	return result, runner.SentTxs(), nil
}

type catalystTx struct {
	hash    string
	lookups int
}

// AwaitPacketsFromCatalyst queries the relayer until each broadcast transaction
// is indexed, then waits for every packet it emitted to stay succeeded.
func AwaitPacketsFromCatalyst(
	ctx context.Context,
	tb testing.TB,
	route e2etest.Route,
	relayer *ibccli.Relayer,
	txs []*catalystevm.SentTx,
) error {
	tb.Helper()

	packets, err := discoverCatalystPackets(ctx, tb, route, relayer, txs, catalystPacketPoll)
	if err != nil {
		return err
	}

	tb.Logf("AwaitPacketsFromCatalyst[%s]: awaiting for %d packets finalization", route.Source, len(packets))
	start := time.Now()

	errs := make([]error, len(packets))
	var wg sync.WaitGroup
	for i, packet := range packets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = e2etest.AwaitStable(ctx, relayer, packet, relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
		}()
	}

	wg.Wait()

	elapsed := time.Since(start)
	tb.Logf("AwaitPacketsFromCatalyst[%s]: %d packets awaited in %s", route.Source, len(packets), elapsed.String())

	return errors.Join(errs...)
}

func discoverCatalystPackets(
	ctx context.Context,
	tb testing.TB,
	route e2etest.Route,
	relayer relayerAPI,
	txs []*catalystevm.SentTx,
	interval time.Duration,
) ([]e2etest.PacketTx, error) {
	pendingTXs, failures := queueCatalystTxs(txs)
	if len(pendingTXs) == 0 {
		return nil, joinDiscoverErrors(failures)
	}

	foundPackets := make([]e2etest.PacketTx, 0, len(pendingTXs))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for len(pendingTXs) > 0 {
		tb.Logf(
			"discoverCatalystPackets[%s]: %d txs to be indexed by the relayer",
			route.Source,
			len(pendingTXs),
		)

		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf(
				"await packets: %d txs still unindexed: %w",
				len(pendingTXs),
				errors.Join(append(failures, err)...),
			)
		}

		nextPendingTXs := make([]catalystTx, 0, len(pendingTXs))
		for _, tx := range pendingTXs {
			txPackets, err := relayer.PacketStatuses(ctx, string(route.Source), tx.hash)
			if err != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}

			if err != nil || len(txPackets) == 0 {
				tx.lookups++
				if tx.lookups >= catalystTxLookups {
					failures = append(failures, undiscoveredTx(tx.hash, err))
					continue
				}
				nextPendingTXs = append(nextPendingTXs, tx)
				continue
			}

			for _, status := range txPackets {
				foundPackets = append(foundPackets, e2etest.PacketTx{
					RouteID:        route.ID,
					Source:         route.Source,
					SourceClientID: status.GetSourceClientId(),
					SourceTxHash:   tx.hash,
					Sequence:       status.GetSequenceNumber(),
				})
			}
		}

		pendingTXs = nextPendingTXs
		if len(pendingTXs) == 0 {
			tb.Logf(
				"discoverCatalystPackets[%s]: %d packets discovered",
				route.Source,
				len(foundPackets),
			)
			break
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf(
				"await catalyst packets: %d txs still unindexed: %w",
				len(pendingTXs),
				errors.Join(append(failures, ctx.Err())...),
			)
		case <-ticker.C:
		}
	}

	if err := joinDiscoverErrors(failures); err != nil {
		return nil, err
	}

	return foundPackets, nil
}

func undiscoveredTx(hash string, err error) error {
	if err != nil {
		return fmt.Errorf("tx %s: %w", hash, err)
	}
	return fmt.Errorf("tx %s: not indexed after %d queries", hash, catalystTxLookups)
}

func joinDiscoverErrors(failed []error) error {
	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf("await catalyst packets: %w", errors.Join(failed...))
}

func queueCatalystTxs(txs []*catalystevm.SentTx) ([]catalystTx, []error) {
	queued := make([]catalystTx, 0, len(txs))
	var failed []error
	for _, tx := range txs {
		if tx == nil {
			continue
		}
		hash := tx.TxHash.Hex()
		if tx.SendTransactionErr != nil {
			failed = append(failed, fmt.Errorf("tx %s was not broadcast: %w", hash, tx.SendTransactionErr))
			continue
		}
		queued = append(queued, catalystTx{hash: hash})
	}
	return queued, failed
}
