// SPDX-License-Identifier: Apache-2.0

// Package relayer re-exports github.com/lfdt-ibc/ibc/cli/api/v2/relayer under
// its pre-rename import path. It covers the identifiers that
// github.com/skip-mev/catalyst uses; add more if a dependency needs them.
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
