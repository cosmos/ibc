package cosmos

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	gmptypes "github.com/cosmos/ibc-go/v11/modules/apps/27-gmp/types"
	ifttypes "github.com/cosmos/ibc-go/v11/modules/apps/prototypes/ift/types"
	channelv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

// This file decodes native Cosmos IFT and 27-gmp send_packet events.

// DiscoveredIFT contains the packet fields recovered from one native source transfer transaction.
type DiscoveredIFT struct {
	ClientID string
	Seq      uint64
	Receiver string
	Amount   *big.Int
	Target   string
	Payload  []byte
	TxHash   string
}

// DiscoverIFTSent returns successful native IFT transfers emitted for sourceClient and denom.
func (c *Client) DiscoverIFTSent(ctx context.Context, sourceClient, denom string) ([]DiscoveredIFT, error) {
	var out []DiscoveredIFT
	query := ifttypes.EventTypeIFTTransferInitiated + "." + ifttypes.AttributeKeyClientID + "='" + sourceClient + "'"
	err := c.forEachTx(ctx, query, func(tx *coretypes.ResultTx) (bool, error) {
		discovered, err := discoverIFTSent(tx, sourceClient, denom)
		if err != nil {
			return false, err
		}
		out = append(out, discovered...)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DiscoveredGMP contains the packet fields recovered from one 27-gmp send event.
type DiscoveredGMP struct {
	Seq     uint64
	Target  string
	Payload []byte
	TxHash  string
}

// DiscoverGMPSent returns GMP packets emitted by sourceClient.
func (c *Client) DiscoverGMPSent(ctx context.Context, sourceClient string) ([]DiscoveredGMP, error) {
	query := channelv2.EventTypeSendPacket + "." + channelv2.AttributeKeySrcClient + "='" + sourceClient + "'"
	var out []DiscoveredGMP
	err := c.forEachTx(ctx, query, func(tx *coretypes.ResultTx) (bool, error) {
		discovered, err := discoverGMPSent(tx, sourceClient)
		if err != nil {
			return false, err
		}
		out = append(out, discovered...)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DiscoverSentTx decodes source packets from one committed transaction.
func (c *Client) DiscoverSentTx(
	ctx context.Context,
	txHash string,
	sourceClient string,
	denom string,
) ([]DiscoveredIFT, []DiscoveredGMP, error) {
	raw, err := hex.DecodeString(txHash)
	if err != nil {
		return nil, nil, fmt.Errorf("cosmos: decode tx hash %q: %w", txHash, err)
	}
	tx, err := c.comet.Tx(ctx, raw, false)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("cosmos: fetch source tx %s: %w", txHash, err)
	}

	ifts, err := discoverIFTSent(tx, sourceClient, denom)
	if err != nil {
		return nil, nil, err
	}

	var gmps []DiscoveredGMP
	if sourceClient != "" {
		gmps, err = discoverGMPSent(tx, sourceClient)
		if err != nil {
			return nil, nil, err
		}
	}
	return ifts, gmps, nil
}

func discoverIFTSent(tx *coretypes.ResultTx, sourceClient, denom string) ([]DiscoveredIFT, error) {
	if tx.TxResult.Code != 0 {
		return nil, nil
	}
	type sentPacket struct {
		target  string
		payload []byte
	}
	sentPackets := make(map[uint64]sentPacket)
	for _, ev := range tx.TxResult.Events {
		if ev.Type != channelv2.EventTypeSendPacket {
			continue
		}
		attrs := attrMap(ev)
		if attrs[channelv2.AttributeKeySrcClient] != sourceClient {
			continue
		}
		target, payload, seq, ok, err := decodeSentPacket(attrs[channelv2.AttributeKeyEncodedPacketHex])
		if err != nil {
			return nil, err
		}
		if ok {
			sentPackets[seq] = sentPacket{target: target, payload: payload}
		}
	}

	var out []DiscoveredIFT
	for _, ev := range tx.TxResult.Events {
		if ev.Type != ifttypes.EventTypeIFTTransferInitiated {
			continue
		}
		attrs := attrMap(ev)
		if attrs[ifttypes.AttributeKeyClientID] != sourceClient || attrs[ifttypes.AttributeKeyDenom] != denom {
			continue
		}
		seq, err := strconv.ParseUint(attrs[ifttypes.AttributeKeySequence], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("cosmos: parse IFT sequence %q: %w", attrs[ifttypes.AttributeKeySequence], err)
		}
		sent, ok := sentPackets[seq]
		if !ok {
			return nil, fmt.Errorf("cosmos: IFT transfer seq %d emitted no matching send_packet", seq)
		}
		amount, ok := new(big.Int).SetString(attrs[ifttypes.AttributeKeyAmount], 10)
		if !ok || amount.Sign() <= 0 {
			return nil, fmt.Errorf("cosmos: invalid IFT amount %q", attrs[ifttypes.AttributeKeyAmount])
		}
		out = append(out, DiscoveredIFT{
			ClientID: sourceClient,
			Seq:      seq,
			Receiver: attrs[ifttypes.AttributeKeyReceiver],
			Amount:   amount,
			Target:   sent.target,
			Payload:  sent.payload,
			TxHash:   tx.Hash.String(),
		})
	}
	return out, nil
}

func discoverGMPSent(tx *coretypes.ResultTx, sourceClient string) ([]DiscoveredGMP, error) {
	if tx.TxResult.Code != 0 {
		return nil, nil
	}
	var out []DiscoveredGMP
	for _, ev := range tx.TxResult.Events {
		if ev.Type != channelv2.EventTypeSendPacket {
			continue
		}
		attrs := attrMap(ev)
		if attrs[channelv2.AttributeKeySrcClient] != sourceClient {
			continue
		}
		target, payload, seq, ok, err := decodeSentPacket(attrs[channelv2.AttributeKeyEncodedPacketHex])
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, DiscoveredGMP{
				Seq: seq, Target: target, Payload: payload, TxHash: tx.Hash.String(),
			})
		}
	}
	return out, nil
}

// decodeSentPacket decodes a channel-v2 send_packet event's encoded_packet_hex into the packet, then reads
// the GMP packet data off its first payload — the EVM receiver (target) and the calldata (payload) — plus the
// module-assigned sequence. ok is false when the packet carries no GMP-port payload.
func decodeSentPacket(encodedHex string) (target string, payload []byte, seq uint64, ok bool, err error) {
	raw, err := hex.DecodeString(encodedHex)
	if err != nil {
		return "", nil, 0, false, fmt.Errorf("cosmos: decode send_packet hex: %w", err)
	}
	var pkt channelv2.Packet
	if uerr := pkt.Unmarshal(raw); uerr != nil {
		return "", nil, 0, false, fmt.Errorf("cosmos: unmarshal send_packet: %w", uerr)
	}
	for _, p := range pkt.Payloads {
		if p.SourcePort != gmptypes.PortID {
			continue
		}
		gpd, derr := gmptypes.UnmarshalPacketData(p.Value, p.Version, p.Encoding)
		if derr != nil {
			return "", nil, 0, false, fmt.Errorf("cosmos: unmarshal GMP packet data: %w", derr)
		}
		return gpd.Receiver, gpd.Payload, pkt.Sequence, true, nil
	}
	return "", nil, 0, false, nil
}
