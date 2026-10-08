// SPDX-License-Identifier: Apache-2.0

package loadtest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethrunner "github.com/skip-mev/catalyst/chains/ethereum/runner"
	catalystevm "github.com/skip-mev/catalyst/chains/ethereum/types"
	catalyst "github.com/skip-mev/catalyst/chains/types"
	"github.com/skip-mev/catalyst/ift/accounts"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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

const (
	msgTypeIFTTransfer = "MsgIFTTransfer"
	packetTimeout      = time.Hour

	// wei value
	defaultTokenTransferAmount = "10000"
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
