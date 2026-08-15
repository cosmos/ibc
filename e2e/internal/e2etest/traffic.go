// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

type RouteID string

// PacketTx locates one protocol packet originated by a test application: the
// route it travels, the source transaction that emitted it, and its sequence
// within that transaction's client. It carries no packet wire data.
type PacketTx struct {
	RouteID           RouteID
	Source            environment.ChainID
	SourceClientID    string
	SourceTxHash      string
	SourceBlockNumber uint64
	Sequence          uint64
}

// String renders the route-scoped label used in relayer status messages.
func (p PacketTx) String() string {
	return fmt.Sprintf("%s-%d", p.RouteID, p.Sequence)
}

func (p PacketTx) reference() string {
	return fmt.Sprintf("route %q sequence %d", p.RouteID, p.Sequence)
}

// sendResult is what every application send produces: the locator of the packet
// it emitted and the source transaction receipt that emitted it. The
// application send types embed it so they all expose the same handles.
type sendResult struct {
	packetTx PacketTx
	receipt  *types.Receipt
}

// newSendResult pairs a source transaction receipt with one packet it emitted.
func newSendResult(
	routeID RouteID,
	source endpoint,
	sourceClientID string,
	receipt *types.Receipt,
	sequence uint64,
) sendResult {
	return sendResult{
		packetTx: PacketTx{
			RouteID:           routeID,
			Source:            source.chain.ID(),
			SourceClientID:    sourceClientID,
			SourceTxHash:      receipt.TxHash.Hex(),
			SourceBlockNumber: receipt.BlockNumber.Uint64(),
			Sequence:          sequence,
		},
		receipt: receipt,
	}
}

// PacketTx locates the packet this send emitted.
func (s sendResult) PacketTx() PacketTx { return s.packetTx }

// TxHash is the hex hash of the source transaction that emitted the packet.
func (s sendResult) TxHash() string { return s.packetTx.SourceTxHash }

// Receipt is the source transaction receipt, for assertions the typed helpers
// do not cover.
func (s sendResult) Receipt() *types.Receipt { return s.receipt }

type endpoint struct {
	chain *environment.Chain
	evm   *environment.EVM
}

func resolveRouteEndpoints(
	routeID RouteID,
	source, destination *environment.Chain,
) (endpoint, endpoint, error) {
	if routeID == "" {
		return endpoint{}, endpoint{}, errors.New("e2etest: route id is required")
	}
	if source == nil {
		return endpoint{}, endpoint{}, errors.New("e2etest: source Chain is required")
	}
	if destination == nil {
		return endpoint{}, endpoint{}, errors.New("e2etest: destination Chain is required")
	}
	if source.ID() == destination.ID() {
		return endpoint{}, endpoint{}, fmt.Errorf(
			"e2etest: route %q must connect different Chains",
			routeID,
		)
	}
	sourceEVM, err := source.EVM()
	if err != nil {
		return endpoint{}, endpoint{}, fmt.Errorf("e2etest: source Chain %q: %w", source.ID(), err)
	}
	destinationEVM, err := destination.EVM()
	if err != nil {
		return endpoint{}, endpoint{}, fmt.Errorf(
			"e2etest: destination Chain %q: %w",
			destination.ID(),
			err,
		)
	}
	return endpoint{chain: source, evm: sourceEVM}, endpoint{chain: destination, evm: destinationEVM}, nil
}
