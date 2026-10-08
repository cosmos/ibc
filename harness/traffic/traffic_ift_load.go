// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/harness/ibccli"
)

// The helpers in this file let an open-loop load generator drive IFT traffic
// without IFT.Send's per-call receipt wait: it builds the same iftTransfer
// calldata, signs and submits raw transactions itself, and turns the receipts
// it collects back into PacketTx locators.

// SignTx signs tx for the EVM chain chainID with the signer's key, which stays
// private to this package.
func (s Signer) SignTx(chainID *big.Int, tx *types.Transaction) (*types.Transaction, error) {
	if s.key == nil {
		return nil, errors.New("traffic: signer has no key")
	}
	if chainID == nil || chainID.Sign() <= 0 {
		return nil, errors.New("traffic: chain id is required to sign")
	}
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), s.key)
	if err != nil {
		return nil, fmt.Errorf("traffic: sign transaction: %w", err)
	}
	return signed, nil
}

// SourceContract is the source IFT token: the target of transfer transactions.
func (i *IFT) SourceContract() common.Address { return i.sourceIFT }

// TransferCalldata packs the iftTransfer call Send submits, for a caller that
// signs and submits the transaction itself to SourceContract.
func (i *IFT) TransferCalldata(receiver common.Address, amount *big.Int, timeoutTimestamp uint64) ([]byte, error) {
	return i.transferCalldata(receiver, amount, timeoutTimestamp)
}

// PacketTxsFromReceipt locates every packet a successful source transaction
// emitted through this route's router, in log order.
func (i *IFT) PacketTxsFromReceipt(receipt *types.Receipt) ([]PacketTx, error) {
	if receipt == nil {
		return nil, errors.New("traffic: receipt is required")
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("traffic: transaction %s failed (status %d)", receipt.TxHash.Hex(), receipt.Status)
	}
	if receipt.BlockNumber == nil {
		return nil, fmt.Errorf("traffic: receipt for %s has no block number", receipt.TxHash.Hex())
	}
	sequences, err := sendPacketSequences(i.sourceRouter, receipt)
	if err != nil {
		return nil, err
	}
	packets := make([]PacketTx, len(sequences))
	for k, sequence := range sequences {
		packets[k] = newSendResult(i.routeID, i.source, i.sourceClientID, receipt, sequence).PacketTx()
	}
	return packets, nil
}

// IsTerminalState reports whether the relayer will not move a packet out of state.
func IsTerminalState(state relayerv2.PacketState) bool {
	switch state {
	case relayerv2.PacketState_PACKET_STATE_SUCCEEDED,
		relayerv2.PacketState_PACKET_STATE_TIMED_OUT,
		relayerv2.PacketState_PACKET_STATE_REJECTED,
		relayerv2.PacketState_PACKET_STATE_RELAY_FAILED:
		return true
	default:
		return false
	}
}

// AwaitTerminalWithPolicy waits under policy, instead of the route's own wait
// policy, until the relayer reports the packet in a terminal state, and
// returns that state. Unlike AwaitState it does not keep polling a packet
// that settled in a state other than the one the caller hopes for, so a
// failed packet costs one observation rather than the whole budget.
func AwaitTerminalWithPolicy(
	ctx context.Context,
	relayer *ibccli.Relayer,
	packet PacketTx,
	policy ibccli.WaitPolicy,
) (relayerv2.PacketState, *relayerv2.PacketStatus, error) {
	if relayer == nil {
		return relayerv2.PacketState_PACKET_STATE_UNSPECIFIED, nil, errors.New("traffic: relayer is required")
	}
	if policy.CompletionBudget <= 0 || policy.StatusPoll <= 0 {
		return relayerv2.PacketState_PACKET_STATE_UNSPECIFIED, nil, fmt.Errorf(
			"traffic: wait policy for packet %s needs a positive budget and poll", packet,
		)
	}
	type settled struct {
		state  relayerv2.PacketState
		status *relayerv2.PacketStatus
	}
	result, err := await(
		ctx,
		policy.CompletionBudget,
		policy.StatusPoll,
		fmt.Sprintf("packet %s to reach a terminal relayer status", packet),
		func(ctx context.Context) (settled, bool, error) {
			observed, state, ok, err := observeStatus(ctx, relayer, packet)
			switch {
			case err != nil:
				return settled{}, false, err
			case !ok:
				return settled{}, false, fmt.Errorf("packet %s is absent from relayer status", packet)
			case !IsTerminalState(state):
				return settled{}, false, fmt.Errorf("packet %s is %q", packet, state)
			}
			if err := verifySourceTx(packet, observed); err != nil {
				return settled{}, true, err
			}
			if err := validateTerminalStatus(packet, state, observed); err != nil {
				return settled{}, true, err
			}
			return settled{state: state, status: observed}, true, nil
		},
	)
	if err != nil {
		return relayerv2.PacketState_PACKET_STATE_UNSPECIFIED, nil, err
	}
	return result.state, result.status, nil
}
