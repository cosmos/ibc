// SPDX-License-Identifier: Apache-2.0

// Package traffic binds test applications and routes above an Environment:
// it deploys the ERC20, Counter, and IFT applications, writes the IBC CLI
// relayer configuration, starts the relayer, and sends ICS20, ICS27 GMP, and
// IFT packets whose relayer status and effects it observes. It does not depend
// on the testing package; resources it starts are returned to the caller, who
// stops them, and files it writes live under caller-owned directories.
package traffic

import (
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

// SendResult is what every application send produces: the locator of the packet
// it emitted and the source transaction receipt that emitted it. The
// application send types embed it so they all expose the same handles.
type SendResult struct {
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
) SendResult {
	return SendResult{
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
func (s SendResult) PacketTx() PacketTx { return s.packetTx }

// TxHash is the hex hash of the source transaction that emitted the packet.
func (s SendResult) TxHash() string { return s.packetTx.SourceTxHash }

// Receipt is the source transaction receipt, for assertions the typed helpers
// do not cover.
func (s SendResult) Receipt() *types.Receipt { return s.receipt }

type endpoint struct {
	chain *environment.Chain
	evm   *environment.EVM
}
