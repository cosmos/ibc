// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/relay/prover"
	"github.com/cosmos/ibc/cli/internal/relay/txbuilder"
	"github.com/cosmos/ibc/cli/internal/txsubmitter"
)

// Route identifies the client pair a pipeline relays.
type Route struct {
	SourceChainID       string
	SourceClientID      string
	DestinationChainID  string
	DestinationClientID string
}

// RouteEnd one chain of a route and what the relayer uses on it.
type RouteEnd struct {
	Chain chains.Client
	// Prover proves counterparty state to this chain: it is this chain's
	// light client of the other end, so the destination's prover proves the
	// source's packets.
	Prover      prover.Prover
	TxBuilder   txbuilder.TxBuilder
	TxSubmitter txsubmitter.TxSubmitter
}

// RouteDeps everything a route's processors relay through, resolved once per
// pipeline.
type RouteDeps struct {
	Route       Route
	Source      RouteEnd
	Destination RouteEnd
}
