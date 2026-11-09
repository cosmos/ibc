// SPDX-License-Identifier: Apache-2.0

// Package relayer exposes github.com/lfdt-ibc/ibc/cli/api/v2/relayer under its
// pre-rename import path, github.com/cosmos/ibc/cli/api/v2/relayer.
//
// It breaks the dependency loop e2e -> skip-mev/catalyst -> this repository's
// CLI API: catalyst still imports the old path, and linking a second, released
// copy of the generated code panics on duplicate proto registration. Every
// identifier is an alias, so the renamed package's types are used and
// registered once. See ../../../README.md.
//
// Only the identifiers catalyst uses are listed; add an alias if a catalyst
// upgrade needs another.
package relayer

import "github.com/lfdt-ibc/ibc/cli/api/v2/relayer"

type (
	AllPackets              = relayer.AllPackets
	PacketSelection         = relayer.PacketSelection
	RelayRequest            = relayer.RelayRequest
	RelayRequest_AllPackets = relayer.RelayRequest_AllPackets
	RelayResponse           = relayer.RelayResponse
	RelayerApiServiceClient = relayer.RelayerApiServiceClient
)

const PacketSelection_PACKET_SELECTION_SELECTED = relayer.PacketSelection_PACKET_SELECTION_SELECTED

var NewRelayerApiServiceClient = relayer.NewRelayerApiServiceClient
