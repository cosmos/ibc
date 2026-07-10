package cosmos

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkbech32 "github.com/cosmos/cosmos-sdk/types/bech32"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	gogoproto "github.com/cosmos/gogoproto/proto"
	gmptypes "github.com/cosmos/ibc-go/v11/modules/apps/27-gmp/types"
	ifttypes "github.com/cosmos/ibc-go/v11/modules/apps/prototypes/ift/types"
	tokenfactorytypes "github.com/cosmos/ibc-go/v11/modules/apps/prototypes/tokenfactory/types"
	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
	channelv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
)

const iftSubdenom = "ift"

var nativeIFTInitialSupply = sdkmath.NewIntFromBigInt(mustBigInt("1000000000000000000000000"))

func mustBigInt(value string) *big.Int {
	n, ok := new(big.Int).SetString(value, 10)
	if !ok {
		panic(fmt.Sprintf("cosmos: invalid integer %q", value))
	}
	return n
}

// CreateIFTDenom creates the native tokenfactory denom and mints the test supply to the user account.
func (c *Client) CreateIFTDenom(ctx context.Context, faucet string) (denom, txHash string, err error) {
	denom = fmt.Sprintf("factory/%s/%s", c.signer.address, iftSubdenom)
	coin := sdk.NewCoin(denom, nativeIFTInitialSupply)
	msgs := []sdk.Msg{
		&tokenfactorytypes.MsgCreateDenom{Sender: c.signer.address, Denom: iftSubdenom},
		&tokenfactorytypes.MsgMint{From: c.signer.address, Address: faucet, Amount: coin},
	}
	txHash, _, err = c.submitMsgs(ctx, msgs)
	if err != nil {
		return "", "", fmt.Errorf("cosmos: create native IFT denom: %w", err)
	}
	return denom, txHash, nil
}

// RegisterIFTBridge binds the native denom and local IBC client to the counterparty IFT contract.
func (c *Client) RegisterIFTBridge(
	ctx context.Context,
	denom, clientID, counterpartyIFT string,
) (string, error) {
	msg := &ifttypes.MsgRegisterIFTBridge{
		Signer:                 c.signer.address,
		Denom:                  denom,
		ClientId:               clientID,
		CounterpartyIftAddress: counterpartyIFT,
		IftSendCallConstructor: ifttypes.ConstructorEVM,
	}
	hash, _, err := c.submitMsgs(ctx, []sdk.Msg{msg})
	if err != nil {
		return "", fmt.Errorf("cosmos: register native IFT bridge: %w", err)
	}
	return hash, nil
}

// DeliverIFT receives an EVM IFT transfer through the real IBC v2 and ICS-27-GMP path. The packet executes
// MsgIFTMint as the derived ICS-27 account; the channel receipt and acknowledgement are the replay guard.
func (c *Client) DeliverIFT(
	ctx context.Context,
	destClient, denom, counterpartyIFT, receiver string,
	amount *big.Int,
	seq uint64,
) (txHash string, success bool, reason string, err error) {
	if h, events, ok, findErr := c.findAckTx(ctx, destClient, seq); findErr != nil {
		return "", false, "", findErr
	} else if ok {
		return classifyIFTAck(h, events, destClient, seq)
	}

	packet, err := buildIFTPacket(destClient, denom, counterpartyIFT, receiver, amount, seq)
	if err != nil {
		return "", false, "", err
	}
	proof, err := buildAttestationProof(packet)
	if err != nil {
		return "", false, "", err
	}
	msg := channelv2.NewMsgRecvPacket(
		packet, proof, clienttypes.NewHeight(0, attestationsClientHeight), c.signer.address,
	)
	hash, events, err := c.submitMsgs(ctx, []sdk.Msg{msg})
	if err != nil {
		return "", false, "", fmt.Errorf("cosmos: submit IFT MsgRecvPacket (seq %d): %w", seq, err)
	}
	return classifyIFTAck(hash, events, destClient, seq)
}

func buildIFTPacket(
	destClient, denom, counterpartyIFT, receiver string,
	amount *big.Int,
	seq uint64,
) (channelv2.Packet, error) {
	accountID := gmptypes.NewAccountIdentifier(destClient, counterpartyIFT, nil)
	ics27, err := gmptypes.BuildAddressPredictable(&accountID)
	if err != nil {
		return channelv2.Packet{}, fmt.Errorf("cosmos: derive IFT ICS27 account: %w", err)
	}
	inner, err := gmptypes.SerializeCosmosTx(cdc, []gogoproto.Message{&ifttypes.MsgIFTMint{
		Signer:   ics27.String(),
		Denom:    denom,
		Receiver: receiver,
		Amount:   sdkmath.NewIntFromBigInt(amount),
	}})
	if err != nil {
		return channelv2.Packet{}, fmt.Errorf("cosmos: serialize MsgIFTMint: %w", err)
	}
	moduleAddress, err := sdkbech32.ConvertAndEncode(Bech32HRP, authtypes.NewModuleAddress(ifttypes.ModuleName))
	if err != nil {
		return channelv2.Packet{}, fmt.Errorf("cosmos: encode IFT module address: %w", err)
	}
	gpd := gmptypes.NewGMPPacketData(counterpartyIFT, moduleAddress, nil, inner, "")
	value, err := gmptypes.MarshalPacketData(&gpd, gmptypes.Version, gmptypes.EncodingABI)
	if err != nil {
		return channelv2.Packet{}, fmt.Errorf("cosmos: marshal IFT GMP packet data: %w", err)
	}
	payload := channelv2.NewPayload(
		gmptypes.PortID,
		gmptypes.PortID,
		gmptypes.Version,
		gmptypes.EncodingABI,
		value,
	)
	return channelv2.NewPacket(
		seq,
		evmCounterpartyClientID,
		destClient,
		uint64(time.Now().Add(packetTimeoutWindow).Unix()),
		payload,
	), nil
}

func classifyIFTAck(
	hash string,
	events []abci.Event,
	destClient string,
	seq uint64,
) (string, bool, string, error) {
	found, success, err := ackSuccessFromEvents(events, destClient, seq)
	if err != nil {
		return "", false, "", err
	}
	if !found {
		return "", false, "", fmt.Errorf("cosmos: IFT recv tx %s wrote no acknowledgement for seq %d", hash, seq)
	}
	if success {
		return hash, true, "", nil
	}
	return hash, false, gmpErrorReason(events), nil
}

// AcknowledgeIFT relays a success acknowledgement back to a native Cosmos IFT source. It uses the exact
// packet emitted by the source transaction and an attestations membership proof for the fabricated EVM ack.
func (c *Client) AcknowledgeIFT(
	ctx context.Context,
	sourceTxHash, sourceClient string,
	seq uint64,
) (string, error) {
	if hash, found, err := c.findIFTCompleted(ctx, sourceClient, seq); err != nil {
		return "", err
	} else if found {
		return hash, nil
	}
	packet, err := c.sourcePacket(ctx, sourceTxHash, sourceClient, seq)
	if err != nil {
		return "", err
	}
	if len(packet.Payloads) != 1 || packet.Payloads[0].SourcePort != gmptypes.PortID {
		return "", fmt.Errorf("cosmos: IFT source packet %s/%d has no single GMP payload", sourceClient, seq)
	}
	ackData := gmptypes.NewAcknowledgement([]byte{1})
	gmpAck, err := gmptypes.MarshalAcknowledgement(
		&ackData, gmptypes.Version, packet.Payloads[0].Encoding,
	)
	if err != nil {
		return "", fmt.Errorf("cosmos: marshal GMP success acknowledgement: %w", err)
	}
	ack := channelv2.NewAcknowledgement(gmpAck)
	proof, err := buildMembershipAttestation(
		hostv2.PacketAcknowledgementKey(packet.DestinationClient, packet.Sequence),
		channelv2.CommitAcknowledgement(ack),
	)
	if err != nil {
		return "", err
	}
	msg := channelv2.NewMsgAcknowledgement(
		packet,
		ack,
		proof,
		clienttypes.NewHeight(0, attestationsClientHeight),
		c.signer.address,
	)
	hash, events, err := c.submitMsgs(ctx, []sdk.Msg{msg})
	if err != nil {
		return "", fmt.Errorf("cosmos: submit IFT acknowledgement (seq %d): %w", seq, err)
	}
	if !hasIFTCompleted(events, sourceClient, seq) {
		return "", fmt.Errorf("cosmos: acknowledgement tx %s emitted no IFT completion for seq %d", hash, seq)
	}
	return hash, nil
}

func (c *Client) sourcePacket(
	ctx context.Context,
	txHash, sourceClient string,
	seq uint64,
) (channelv2.Packet, error) {
	raw, err := hex.DecodeString(txHash)
	if err != nil {
		return channelv2.Packet{}, fmt.Errorf("cosmos: decode source tx hash %q: %w", txHash, err)
	}
	tx, err := c.comet.Tx(ctx, raw, false)
	if err != nil {
		return channelv2.Packet{}, fmt.Errorf("cosmos: fetch source tx %s: %w", txHash, err)
	}
	for _, event := range tx.TxResult.Events {
		if event.Type != channelv2.EventTypeSendPacket {
			continue
		}
		attrs := attrMap(event)
		if attrs[channelv2.AttributeKeySrcClient] != sourceClient {
			continue
		}
		encoded, err := hex.DecodeString(attrs[channelv2.AttributeKeyEncodedPacketHex])
		if err != nil {
			return channelv2.Packet{}, fmt.Errorf("cosmos: decode source packet: %w", err)
		}
		var packet channelv2.Packet
		if err := packet.Unmarshal(encoded); err != nil {
			return channelv2.Packet{}, fmt.Errorf("cosmos: unmarshal source packet: %w", err)
		}
		if packet.Sequence == seq {
			return packet, nil
		}
	}
	return channelv2.Packet{}, fmt.Errorf("cosmos: source tx %s contains no packet %s/%d", txHash, sourceClient, seq)
}

func (c *Client) findIFTCompleted(
	ctx context.Context,
	clientID string,
	seq uint64,
) (hash string, found bool, err error) {
	query := ifttypes.EventTypeIFTTransferCompleted + "." + ifttypes.AttributeKeyClientID + "='" + clientID + "'"
	err = c.forEachTx(ctx, query, func(tx *coretypes.ResultTx) (bool, error) {
		if tx.TxResult.Code == 0 && hasIFTCompleted(tx.TxResult.Events, clientID, seq) {
			hash, found = tx.Hash.String(), true
			return true, nil
		}
		return false, nil
	})
	return hash, found, err
}

func hasIFTCompleted(events []abci.Event, clientID string, seq uint64) bool {
	seqString := fmt.Sprintf("%d", seq)
	for _, event := range events {
		if event.Type != ifttypes.EventTypeIFTTransferCompleted {
			continue
		}
		attrs := attrMap(event)
		if attrs[ifttypes.AttributeKeyClientID] == clientID && attrs[ifttypes.AttributeKeySequence] == seqString {
			return true
		}
	}
	return false
}
