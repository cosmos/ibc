// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"fmt"
	"slices"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
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

// ID identifies the packet among all packets the source Chain sends.
func (p PacketTx) ID() PacketID {
	return PacketID{SourceClientID: p.SourceClientID, Sequence: p.Sequence}
}

// emittedBy checks that ids, read from txHash's events of the given kind,
// include this packet.
func (p PacketTx) emittedBy(ids []PacketID, event, txHash string) error {
	if !slices.Contains(ids, p.ID()) {
		return fmt.Errorf("e2etest: transaction %s emitted no %s event for packet %s", txHash, event, p.reference())
	}
	return nil
}

func (p PacketTx) reference() string {
	return fmt.Sprintf("route %q sequence %d", p.RouteID, p.Sequence)
}

// PacketID identifies a packet on its source Chain: a sequence is only unique
// per source client.
type PacketID struct {
	SourceClientID string
	Sequence       uint64
}

// sendResult is what every application send produces: the locator of the packet
// it emitted and the packet itself. The application send types embed it so they
// all expose the same handles.
type sendResult struct {
	packetTx PacketTx
	packet   ics26router.IICS26RouterMsgsPacket
}

// sourceRoute is the source side every application sends from: the signer on
// the source Chain and the router and client its packets leave through.
type sourceRoute struct {
	routeID        RouteID
	source         endpoint
	sender         evm.Account
	sourceRouter   common.Address
	sourceClientID string
}

// PacketTx locates the packet this send emitted.
func (s sendResult) PacketTx() PacketTx { return s.packetTx }

// Packet is the packet this send emitted, as the source router's SendPacket
// event carried it.
func (s sendResult) Packet() ics26router.IICS26RouterMsgsPacket { return s.packet }

// TxHash is the hex hash of the source transaction that emitted the packet.
func (s sendResult) TxHash() string { return s.packetTx.SourceTxHash }

type endpoint struct {
	chain *environment.Chain
	evm   *environment.EVM
}
