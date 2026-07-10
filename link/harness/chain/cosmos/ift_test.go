package cosmos

import (
	"testing"

	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"
	ifttypes "github.com/cosmos/ibc-go/v11/modules/apps/prototypes/ift/types"
	tokenfactorytypes "github.com/cosmos/ibc-go/v11/modules/apps/prototypes/tokenfactory/types"
	channelv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

func TestNativeIFTTypesRegistered(t *testing.T) {
	registry := protoCodec.InterfaceRegistry()
	require.NoError(t, registry.EnsureRegistered(&ifttypes.MsgIFTTransfer{}))
	require.NoError(t, registry.EnsureRegistered(&tokenfactorytypes.MsgCreateDenom{}))
}

func TestIFTSeqFromEvents(t *testing.T) {
	events := []abci.Event{
		newEvent(ifttypes.EventTypeIFTTransferInitiated, map[string]string{
			ifttypes.AttributeKeyClientID: "attestations-0",
			ifttypes.AttributeKeySequence: "42",
		}),
		newEvent(channelv2.EventTypeSendPacket, map[string]string{
			channelv2.AttributeKeySrcClient: "attestations-0",
			channelv2.AttributeKeySequence:  "42",
		}),
	}

	seq, ok := iftSeqFromEvents(events, "attestations-0")
	require.True(t, ok)
	require.Equal(t, uint64(42), seq)

	events[1] = newEvent(channelv2.EventTypeSendPacket, map[string]string{
		channelv2.AttributeKeySrcClient: "attestations-0",
		channelv2.AttributeKeySequence:  "43",
	})
	_, ok = iftSeqFromEvents(events, "attestations-0")
	require.False(t, ok)
}

func TestIFTMintFromEvents(t *testing.T) {
	events := []abci.Event{newEvent(ifttypes.EventTypeIFTMintReceived, map[string]string{
		ifttypes.AttributeKeyClientID: "attestations-1",
		ifttypes.AttributeKeyReceiver: "cosmos1receiver",
		ifttypes.AttributeKeyAmount:   "340282366920938463463374607431768211456",
		ifttypes.AttributeKeyDenom:    "factory/cosmos1admin/ift",
	})}

	receiver, amount, denom, ok, err := iftMintFromEvents(events, "attestations-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "cosmos1receiver", receiver)
	require.Equal(t, "340282366920938463463374607431768211456", amount.String())
	require.Equal(t, "factory/cosmos1admin/ift", denom)
}

func newEvent(eventType string, attrs map[string]string) abci.Event {
	event := abci.Event{Type: eventType}
	for key, value := range attrs {
		event.Attributes = append(event.Attributes, abci.EventAttribute{Key: key, Value: value})
	}
	return event
}
