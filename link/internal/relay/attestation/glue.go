package attestation

import (
	"context"
	"encoding/hex"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/chains/evm/contracts/ics26router"
	"github.com/cosmos/ibc/link/internal/config"

	ibcattestor "github.com/cosmos/ibc/link/internal/types/ibcattestor"
	proto "github.com/cosmos/ibc/link/internal/types/proofapi"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// resolvedClient the attestor set trusted by one configured light client,
// with its attestors already dialed.
type resolvedClient struct {
	threshold int
	attestors []namedAttestorClient
}

// Glue implements proto.ProofApiServiceClient's RelayByTx in process,
// replacing the external proof api: it resolves the attestor set for the
// relevant client, aggregates quorum-verified attestations, and forms the
// resulting multicall. CreateClient/UpdateClient/Info are not implemented;
// light client deployment stays a separate, external concern.
type Glue struct {
	clients      *chains.ClientSet
	chainRouters map[string]string
	byClient     map[string]resolvedClient
}

var _ proto.ProofApiServiceClient = (*Glue)(nil)

// NewGlue resolves every configured client's attestor set and dials its
// attestors once, up front, so the request path only ever queries them.
func NewGlue(cfg config.Config, clientSet *chains.ClientSet) (*Glue, error) {
	chainRouters := make(map[string]string, len(cfg.Chains))
	for _, chain := range cfg.Chains {
		if chain.EVM != nil {
			chainRouters[chain.ChainID] = chain.EVM.ICS26Router
		}
	}

	byClient := make(map[string]resolvedClient, len(cfg.Relayer.Clients))

	for _, clientCfg := range cfg.Relayer.Clients {
		attestors := make([]namedAttestorClient, 0, len(clientCfg.AttestorSet.Attestors))

		for _, entry := range clientCfg.AttestorSet.Attestors {
			client, err := attestorClient(entry)
			if err != nil {
				return nil, errors.Wrapf(err, "client %q attestor %q", clientCfg.Alias, entry.Name)
			}

			attestors = append(attestors, namedAttestorClient{Name: entry.Name, Client: client})
		}

		byClient[clientKey(clientCfg.ChainID, clientCfg.ClientID)] = resolvedClient{
			threshold: clientCfg.AttestorSet.Threshold,
			attestors: attestors,
		}
	}

	return &Glue{clients: clientSet, chainRouters: chainRouters, byClient: byClient}, nil
}

func clientKey(chainID, clientID string) string {
	return chainID + "/" + clientID
}

// RelayByTx dispatches to recv, ack, or timeout handling based on which
// fields the request populates, matching the shapes the batch processors in
// internal/relay/processors send today.
func (g *Glue) RelayByTx(
	ctx context.Context,
	req *connect.Request[proto.RelayByTxRequest],
) (*connect.Response[proto.RelayByTxResponse], error) {
	msg := req.Msg

	switch {
	case len(msg.GetTimeoutTxIds()) > 0:
		return g.relayTimeout(ctx, msg)
	case len(msg.GetDstPacketSequences()) > 0:
		return g.relayAck(ctx, msg)
	case len(msg.GetSrcPacketSequences()) > 0:
		return g.relayRecv(ctx, msg)
	default:
		return nil, errors.New("relay by tx: request shape matches neither recv, ack, nor timeout")
	}
}

func (g *Glue) CreateClient(
	context.Context,
	*connect.Request[proto.CreateClientRequest],
) (*connect.Response[proto.CreateClientResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("light client deployment is not handled by the relayer"))
}

func (g *Glue) UpdateClient(
	context.Context,
	*connect.Request[proto.UpdateClientRequest],
) (*connect.Response[proto.UpdateClientResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("standalone client updates are not handled by the relayer"))
}

func (g *Glue) Info(
	context.Context,
	*connect.Request[proto.InfoRequest],
) (*connect.Response[proto.InfoResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("info is not implemented by the relayer's proof api glue"))
}

// relayRecv delivers a batch of send packets observed on the source chain to
// the destination chain.
func (g *Glue) relayRecv(ctx context.Context, msg *proto.RelayByTxRequest) (*connect.Response[proto.RelayByTxResponse], error) {
	events, err := g.packetEventsFromTxs(ctx, msg.GetSrcChain(), msg.GetSourceTxIds(), msg.GetSrcPacketSequences())
	if err != nil {
		return nil, err
	}

	resolved, ok := g.byClient[clientKey(msg.GetDstChain(), msg.GetDstClientId())]
	if !ok {
		return nil, errors.Errorf("no attestor set configured for client %q on chain %q", msg.GetDstClientId(), msg.GetDstChain())
	}

	packets := packetsOf(events)

	bundle, err := g.attest(ctx, resolved, packets, msg.GetDstClientId(), ibcattestor.CommitmentType_COMMITMENT_TYPE_PACKET)
	if err != nil {
		return nil, err
	}

	calls := make([][]byte, 0, len(packets)+1)
	calls = append(calls, bundle.updateClientCall)

	for _, packet := range packets {
		call, errPack := packRecvPacket(toRouterPacket(packet), bundle.packetProof, bundle.height)
		if errPack != nil {
			return nil, errors.Wrapf(errPack, "packing recvPacket for sequence %d", packet.Sequence)
		}

		calls = append(calls, call)
	}

	return g.respond(msg.GetDstChain(), calls)
}

// relayAck delivers a batch of write-acknowledgement packets observed on the
// destination chain back to the original source chain.
func (g *Glue) relayAck(ctx context.Context, msg *proto.RelayByTxRequest) (*connect.Response[proto.RelayByTxResponse], error) {
	events, err := g.packetEventsFromTxs(ctx, msg.GetSrcChain(), msg.GetSourceTxIds(), msg.GetDstPacketSequences())
	if err != nil {
		return nil, err
	}

	resolved, ok := g.byClient[clientKey(msg.GetDstChain(), msg.GetDstClientId())]
	if !ok {
		return nil, errors.Errorf("no attestor set configured for client %q on chain %q", msg.GetDstClientId(), msg.GetDstChain())
	}

	bundle, err := g.attest(ctx, resolved, packetsOf(events), msg.GetDstClientId(), ibcattestor.CommitmentType_COMMITMENT_TYPE_ACK)
	if err != nil {
		return nil, err
	}

	calls := make([][]byte, 0, len(events)+1)
	calls = append(calls, bundle.updateClientCall)

	for _, event := range events {
		if len(event.Acks) == 0 {
			return nil, errors.Errorf("no acknowledgement recorded for sequence %d", event.Packet.Sequence)
		}

		call, errPack := packAckPacket(toRouterPacket(event.Packet), event.Acks[0], bundle.packetProof, bundle.height)
		if errPack != nil {
			return nil, errors.Wrapf(errPack, "packing ackPacket for sequence %d", event.Packet.Sequence)
		}

		calls = append(calls, call)
	}

	return g.respond(msg.GetDstChain(), calls)
}

// relayTimeout delivers a batch of timeout packets: packets originally sent
// on the destination chain (in this request's terms) that were never
// received, proven by non-membership of their receipts.
func (g *Glue) relayTimeout(ctx context.Context, msg *proto.RelayByTxRequest) (*connect.Response[proto.RelayByTxResponse], error) {
	events, err := g.packetEventsFromTxs(ctx, msg.GetDstChain(), msg.GetTimeoutTxIds(), msg.GetDstPacketSequences())
	if err != nil {
		return nil, err
	}

	resolved, ok := g.byClient[clientKey(msg.GetDstChain(), msg.GetDstClientId())]
	if !ok {
		return nil, errors.Errorf("no attestor set configured for client %q on chain %q", msg.GetDstClientId(), msg.GetDstChain())
	}

	packets := packetsOf(events)

	bundle, err := g.attest(ctx, resolved, packets, msg.GetDstClientId(), ibcattestor.CommitmentType_COMMITMENT_TYPE_RECEIPT)
	if err != nil {
		return nil, err
	}

	calls := make([][]byte, 0, len(packets)+1)
	calls = append(calls, bundle.updateClientCall)

	for _, packet := range packets {
		call, errPack := packTimeoutPacket(toRouterPacket(packet), bundle.packetProof, bundle.height)
		if errPack != nil {
			return nil, errors.Wrapf(errPack, "packing timeoutPacket for sequence %d", packet.Sequence)
		}

		calls = append(calls, call)
	}

	return g.respond(msg.GetDstChain(), calls)
}

// attestationBundle one quorum-verified state update plus the packet proof
// attested at the same height, ready to pack into router calls.
type attestationBundle struct {
	updateClientCall []byte
	packetProof      []byte
	height           uint64
}

// attest resolves a height every attestor in the set has observed, then
// queries both a state and a packet attestation quorum at that height.
func (g *Glue) attest(
	ctx context.Context,
	resolved resolvedClient,
	packets []v2.Packet,
	dstClientID string,
	kind ibcattestor.CommitmentType,
) (attestationBundle, error) {
	height, err := latestHeight(ctx, resolved.attestors, resolved.threshold)
	if err != nil {
		return attestationBundle{}, errors.Wrap(err, "resolving attested height")
	}

	stateResult, err := queryStateQuorum(ctx, resolved.attestors, resolved.threshold, height)
	if err != nil {
		return attestationBundle{}, errors.Wrap(err, "querying state attestation quorum")
	}

	decodedState, err := decodeStateAttestation(stateResult.AttestationData)
	if err != nil {
		return attestationBundle{}, errors.Wrap(err, "decoding state attestation quorum result")
	}

	if decodedState.Height != height {
		return attestationBundle{}, errors.Errorf("state attestation height %d does not match requested height %d", decodedState.Height, height)
	}

	encodedPackets := make([][]byte, len(packets))

	for i, packet := range packets {
		encoded, errEnc := encodePacket(packet)
		if errEnc != nil {
			return attestationBundle{}, errors.Wrapf(errEnc, "encoding packet sequence %d", packet.Sequence)
		}

		encodedPackets[i] = encoded
	}

	packetResult, err := queryPacketQuorum(ctx, resolved.attestors, resolved.threshold, encodedPackets, height, kind)
	if err != nil {
		return attestationBundle{}, errors.Wrap(err, "querying packet attestation quorum")
	}

	decodedPackets, err := decodePacketAttestation(packetResult.AttestationData)
	if err != nil {
		return attestationBundle{}, errors.Wrap(err, "decoding packet attestation quorum result")
	}

	if decodedPackets.Height != height {
		return attestationBundle{}, errors.Errorf("packet attestation height %d does not match requested height %d", decodedPackets.Height, height)
	}

	if len(decodedPackets.Packets) != len(packets) {
		return attestationBundle{}, errors.Errorf(
			"packet attestation returned %d packets, expected %d", len(decodedPackets.Packets), len(packets),
		)
	}

	updateClientCall, err := packUpdateClient(dstClientID, AttestationProof{
		AttestationData: stateResult.AttestationData,
		Signatures:      stateResult.Signatures,
	})
	if err != nil {
		return attestationBundle{}, err
	}

	packetProof, err := encodeAttestationProof(AttestationProof{
		AttestationData: packetResult.AttestationData,
		Signatures:      packetResult.Signatures,
	})
	if err != nil {
		return attestationBundle{}, errors.Wrap(err, "encoding packet attestation proof")
	}

	return attestationBundle{updateClientCall: updateClientCall, packetProof: packetProof, height: height}, nil
}

// packetEventsFromTxs recovers packet events from a chain's observed events
// for each tx id, keeping only sequences present in the caller-supplied
// filter (the batch may cover multiple sequences per tx, or the filter may
// be empty to keep everything a tx yields).
func (g *Glue) packetEventsFromTxs(
	ctx context.Context,
	chainID string,
	txIDs [][]byte,
	sequenceFilter []uint64,
) ([]v2.PacketEvent, error) {
	client, ok := g.clients.Get(chainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for chain %q", chainID)
	}

	keep := make(map[uint64]struct{}, len(sequenceFilter))
	for _, seq := range sequenceFilter {
		keep[seq] = struct{}{}
	}

	var events []v2.PacketEvent

	for _, txID := range txIDs {
		txEvents, err := client.TxPacketEvents(ctx, txID)
		if err != nil {
			return nil, errors.Wrapf(err, "reading packet events for tx %s", hex.EncodeToString(txID))
		}

		for _, event := range txEvents {
			if len(keep) > 0 {
				if _, ok := keep[event.Packet.Sequence]; !ok {
					continue
				}
			}

			events = append(events, event)
		}
	}

	return events, nil
}

func packetsOf(events []v2.PacketEvent) []v2.Packet {
	packets := make([]v2.Packet, len(events))
	for i, event := range events {
		packets[i] = event.Packet
	}

	return packets
}

func (g *Glue) respond(dstChain string, calls [][]byte) (*connect.Response[proto.RelayByTxResponse], error) {
	tx, err := packMulticall(calls)
	if err != nil {
		return nil, err
	}

	address, ok := g.chainRouters[dstChain]
	if !ok {
		return nil, errors.Errorf("no router address configured for chain %q", dstChain)
	}

	return connect.NewResponse(&proto.RelayByTxResponse{Tx: tx, Address: address}), nil
}

func toRouterPacket(packet v2.Packet) ics26router.IICS26RouterMsgsPacket {
	payloads := make([]ics26router.IICS26RouterMsgsPayload, len(packet.Payloads))
	for i, p := range packet.Payloads {
		payloads[i] = ics26router.IICS26RouterMsgsPayload{
			SourcePort: p.SourcePort,
			DestPort:   p.DestPort,
			Version:    p.Version,
			Encoding:   p.Encoding,
			Value:      p.Value,
		}
	}

	return ics26router.IICS26RouterMsgsPacket{
		Sequence:         packet.Sequence,
		SourceClient:     packet.SourceClient,
		DestClient:       packet.DestClient,
		TimeoutTimestamp: packet.TimeoutTimestamp,
		Payloads:         payloads,
	}
}
