// Package submit implements chain.AppSubmitter for EVM fixture contracts.
package submit

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/fixtures"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// The fixture ABIs are fixed at build time, so a parse failure is a packaging bug.
var (
	mockIFTABI = fixtures.MockIFT.MustABI()
	mockGMPABI = fixtures.MockGMP.MustABI()
)

// Submitter sends fixture transactions on one EVM source chain.
type Submitter struct {
	client *evm.EVMClient
	ift    common.Address
	gmp    common.Address
}

var _ chain.AppSubmitter = (*Submitter)(nil)

// New resolves the fixture addresses for an EVM source chain.
func New(c *evm.EVMClient, dep wire.ChainDeployment) (*Submitter, error) {
	ift, err := fixtureAddr(dep, fixturekeys.MockIFT)
	if err != nil {
		return nil, err
	}
	gmp, err := fixtureAddr(dep, fixturekeys.MockGMP)
	if err != nil {
		return nil, err
	}
	return &Submitter{client: c, ift: ift, gmp: gmp}, nil
}

// SubmitIFT sends MockIFT.sendTransfer and returns its transaction hash and IFTSent sequence.
func (s *Submitter) SubmitIFT(ctx context.Context, in chain.IFTSubmission) (chain.AppSubmitResult, error) {
	data, err := mockIFTABI.Pack(
		"sendTransfer", in.RouteID, in.Receiver, in.Amount, new(big.Int).SetUint64(in.TimeoutTimestamp),
	)
	if err != nil {
		return chain.AppSubmitResult{}, fmt.Errorf("evm submit: pack sendTransfer: %w", err)
	}
	return s.send(ctx, s.ift, data, mockIFTABI, "IFTSent")
}

// SubmitGMP sends MockGMP.send and returns its transaction hash and GMPSent sequence.
func (s *Submitter) SubmitGMP(ctx context.Context, in chain.GMPSubmission) (chain.AppSubmitResult, error) {
	data, err := mockGMPABI.Pack("send", in.RouteID, in.Target, in.Payload)
	if err != nil {
		return chain.AppSubmitResult{}, fmt.Errorf("evm submit: pack send: %w", err)
	}
	return s.send(ctx, s.gmp, data, mockGMPABI, "GMPSent")
}

// send broadcasts a faucet-signed transaction and reads its source sequence from the receipt.
func (s *Submitter) send(
	ctx context.Context,
	to common.Address,
	data []byte,
	parsed abi.ABI,
	sentEvent string,
) (chain.AppSubmitResult, error) {
	rcpt, err := s.client.BroadcastTx(ctx, evm.FaucetAccount(), &to, data, nil)
	if err != nil {
		return chain.AppSubmitResult{}, fmt.Errorf("evm submit: broadcast %s: %w", sentEvent, err)
	}
	seq, ok, err := sentSeqFromReceipt(rcpt, parsed, sentEvent)
	if err != nil {
		return chain.AppSubmitResult{}, err
	}
	if !ok {
		return chain.AppSubmitResult{}, fmt.Errorf(
			"evm submit: source tx %s emitted no %s event",
			rcpt.TxHash.Hex(),
			sentEvent,
		)
	}
	return chain.AppSubmitResult{SourceTxHash: rcpt.TxHash.Hex(), Sequence: seq}, nil
}

// sentSeqFromReceipt reads the leading, non-indexed uint256 sequence from a Sent event.
func sentSeqFromReceipt(rcpt *types.Receipt, parsed abi.ABI, event string) (uint64, bool, error) {
	topic := parsed.Events[event].ID
	for _, lg := range rcpt.Logs {
		if len(lg.Topics) == 0 || lg.Topics[0] != topic {
			continue
		}
		vals, err := parsed.Events[event].Inputs.Unpack(lg.Data)
		if err != nil {
			return 0, false, fmt.Errorf("evm submit: decode %s: %w", event, err)
		}
		seq, ok := vals[0].(*big.Int)
		if !ok {
			return 0, false, fmt.Errorf("evm submit: %s seq field is %T, want *big.Int", event, vals[0])
		}
		return seq.Uint64(), true, nil
	}
	return 0, false, nil
}

// fixtureAddr resolves a fixture without coercing malformed input to the zero address.
func fixtureAddr(dep wire.ChainDeployment, name string) (common.Address, error) {
	s, err := dep.Fixture(name)
	if err != nil {
		return common.Address{}, fmt.Errorf("evm submit: %w", err)
	}
	if !common.IsHexAddress(s) {
		return common.Address{}, fmt.Errorf("evm submit: fixture %s %q is not a valid EVM address", name, s)
	}
	return common.HexToAddress(s), nil
}
