package onchain

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

func TestTypedActionIDUsesApplicationType(t *testing.T) {
	packet := PacketAction{RouteID: "route", Sequence: 7}

	ift := IFTAction{PacketAction: packet}
	gmp := GMPAction{PacketAction: packet}

	require.Equal(t, wire.PacketID("route", wire.AppTypeIFT, 7), ift.ID())
	require.Equal(t, wire.PacketID("route", wire.AppTypeGMP, 7), gmp.ID())
	require.NotEqual(t, ift.ID(), gmp.ID())
}
