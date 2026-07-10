package cosmos

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/cosmos/ibc/link/harness/testkeys"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	txsigning "github.com/cosmos/cosmos-sdk/x/tx/signing"
	gogoproto "github.com/cosmos/gogoproto/proto"
	gmptypes "github.com/cosmos/ibc-go/v11/modules/apps/27-gmp/types"
	ifttypes "github.com/cosmos/ibc-go/v11/modules/apps/prototypes/ift/types"
	tokenfactorytypes "github.com/cosmos/ibc-go/v11/modules/apps/prototypes/tokenfactory/types"
	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
	clientv2types "github.com/cosmos/ibc-go/v11/modules/core/02-client/v2/types"
	channelv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	attestations "github.com/cosmos/ibc-go/v11/modules/light-clients/attestations"
)

// This file is the stub's IBC v2 (Eureka) delivery of GMP messages to a cosmos destination: it drives the
// chain's native ICS-27 (27-gmp) module over a signed MsgRecvPacket, proven by the chain's `attestations`
// light client. The module receives the packet, derives the ICS-27 account, and atomically executes the
// delivered CosmosTx, returning a real success or error acknowledgement. The stub plays the mock EVM source
// and the 1-of-1 attestor for the fabricated packet. The cosmos-source GMP submission (the real
// MsgSendCall) happens on the harness's side of the wall — the relayer discovers it from the module's
// send_packet events (see discover.go); only the delivery leg here and the destination ack/timeout legs
// are in scope.

const (
	// gmpGasLimit is the gas budget for the module-driving txs (client create, counterparty register, and the
	// MsgRecvPacket that runs the ICS-27 inner CosmosTx). Bank sends fit in the default gasLimit; module
	// execution needs far more, so this is a generous fixed ceiling (fees stay zero — min gas price 0astake).
	gmpGasLimit = 1_000_000

	// evmCounterpartyClientID is the fabricated source-side (EVM) client id the cosmos attestations client is
	// told its counterparty is. The EVM side is fully mocked, so this is routing metadata only: it must be a
	// valid IBC client id ({type}-{seq}) and it must equal the packet's SourceClient (recvPacket asserts
	// counterparty.ClientId == packet.SourceClient), and it feeds the commitment key path the attestation signs
	// over. It is not a real client on any chain.
	evmCounterpartyClientID = "evmmock-0"

	// attestationsClientHeight is the fixed nominal height the attestations client is created at and every
	// membership proof is verified at. The mock client needs a consensus state at the proof height and an
	// attestation Height equal to it; it does not compare against real EVM chain state. Both create and
	// delivery pin this value, so every recv proves at one height with no MsgUpdateClient tx.
	attestationsClientHeight = 1

	// packetTimeoutWindow is how far in the future each fabricated packet's TimeoutTimestamp is set. recvPacket
	// rejects a packet whose timeout <= current block time, so this must comfortably exceed one block; an hour
	// is ample and well under the module's MaxTimeoutDelta upper bound.
	packetTimeoutWindow = time.Hour

	// ics27FundGMP is the amount `deploy` funds the ICS-27 executor account with in the GMP counter denom
	// (ugmpc), which bankrolls increments (each moves 1 ugmpc ICS27->target). It is minted far above any
	// test's increment count so funding is never the cause of a failure.
	ics27FundGMP = 1_000_000

	// ics27ErrorAckAmount is the ugmpc the error-ack inner MsgSend attempts to move ICS27->target: astronomically
	// more than the account is funded with, so the module's atomic execution fails with insufficient funds and
	// returns an ERROR acknowledgement, leaving the counter target unchanged (the cached context is discarded).
	ics27ErrorAckAmount = "1000000000000000000000000000000"
)

// cdc and txConfig are the stub's cosmos codec and SIGN_MODE_DIRECT tx builder for signed txs,
// inner-CosmosTx payloads, and attestation proofs. InterfaceRegistryOptions needs an explicit "cosmos"
// bech32 AddressCodec; the SDK default signing codec fails signer extraction from bech32 fields. The registry
// includes secp256k1 keys plus bank and ibc-go message types.
var cdc, txConfig = newCodecAndTxConfig()

func newCodecAndTxConfig() (*codec.ProtoCodec, client.TxConfig) {
	registry, err := codectypes.NewInterfaceRegistryWithOptions(codectypes.InterfaceRegistryOptions{
		ProtoFiles: gogoproto.HybridResolver,
		SigningOptions: txsigning.Options{
			AddressCodec:          address.NewBech32Codec(Bech32HRP),
			ValidatorAddressCodec: address.NewBech32Codec(Bech32HRP + "valoper"),
		},
	})
	if err != nil {
		panic(fmt.Sprintf("cosmos: build interface registry: %v", err))
	}
	cryptocodec.RegisterInterfaces(registry)
	banktypes.RegisterInterfaces(registry)
	clienttypes.RegisterInterfaces(registry)
	clientv2types.RegisterInterfaces(registry)
	channelv2.RegisterInterfaces(registry)
	gmptypes.RegisterInterfaces(registry)
	ifttypes.RegisterInterfaces(registry)
	tokenfactorytypes.RegisterInterfaces(registry)
	attestations.RegisterInterfaces(registry)
	protoCodec := codec.NewProtoCodec(registry)
	return protoCodec, authtx.NewTxConfig(protoCodec, []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_DIRECT})
}

// attestorKey is the stub's single test attestor key (see testkeys.AttestorPrivateKeyHex). It signs every
// packet-commitment attestation the cosmos light client verifies (1-of-1 quorum).
var attestorKey = mustAttestorKey()

func mustAttestorKey() *ecdsa.PrivateKey {
	k, err := crypto.HexToECDSA(strings.TrimPrefix(testkeys.AttestorPrivateKeyHex, "0x"))
	if err != nil {
		panic(fmt.Sprintf("cosmos: parse attestor key: %v", err))
	}
	return k
}

// AttestorAddressHex is the attestor's Ethereum EOA hex — the sole entry in the attestations ClientState's
// attestor set. Derived from attestorKey so the on-chain client and the signer can never disagree.
func AttestorAddressHex() string { return crypto.PubkeyToAddress(attestorKey.PublicKey).Hex() }

// GMPSender is the fixed sender identity for every evm->cosmos GMP message: the EVM faucet's hex address. The
// ICS-27 account the module runs deliveries as is derived from (destination client id, this sender, empty
// salt), so `deploy` (which funds that account) and delivery (which sets GMPPacketData.Sender) must use this
// exact value — it is the shared identity that makes the two agree on the executor account.
const GMPSender = testkeys.FaucetAddressHex

// ICS27Account returns the bech32 of the ICS-27 executor account the 27-gmp module derives for GMPSender under
// destClient with an empty salt — the account `deploy` funds and that runs each delivered CosmosTx. It is a
// pure function of the inputs (deterministic), so `deploy` (emitting it as a fixture) and any reader agree.
func ICS27Account(destClient string) (string, error) {
	accID := gmptypes.NewAccountIdentifier(destClient, GMPSender, nil)
	addr, err := gmptypes.BuildAddressPredictable(&accID)
	if err != nil {
		return "", fmt.Errorf("cosmos: derive ICS27 account: %w", err)
	}
	return addr.String(), nil
}

// CreateAttestationsClient creates the destination chain's `attestations` light client with the stub's single
// attestor (minRequiredSigs=1) at the nominal client height, and returns the created client id (parsed from
// the create_client event, never assumed) plus the committing tx hash. The consensus state's timestamp is
// "now" (non-zero, as the client requires); the height is nominal (see attestationsClientHeight).
func (c *Client) CreateAttestationsClient(ctx context.Context) (clientID, txHash string, err error) {
	clientState := attestations.NewClientState([]string{AttestorAddressHex()}, 1, attestationsClientHeight)
	consensusState := &attestations.ConsensusState{Timestamp: uint64(time.Now().UnixNano())}
	csAny, err := codectypes.NewAnyWithValue(clientState)
	if err != nil {
		return "", "", fmt.Errorf("cosmos: pack client state: %w", err)
	}
	consAny, err := codectypes.NewAnyWithValue(consensusState)
	if err != nil {
		return "", "", fmt.Errorf("cosmos: pack consensus state: %w", err)
	}
	msg := &clienttypes.MsgCreateClient{
		ClientState:    csAny,
		ConsensusState: consAny,
		Signer:         c.signer.address,
	}
	hash, events, err := c.submitMsgs(ctx, []sdk.Msg{msg})
	if err != nil {
		return "", "", fmt.Errorf("cosmos: create attestations client: %w", err)
	}
	id, ok := clientIDFromEvents(events)
	if !ok {
		return "", "", fmt.Errorf(
			"cosmos: create attestations client tx %s emitted no %s.%s event",
			hash,
			clienttypes.EventTypeCreateClient,
			clienttypes.AttributeKeyClientID,
		)
	}
	return id, hash, nil
}

// RegisterCounterparty binds the destination attestations client to the fabricated EVM-side client id with a
// single-element merkle prefix (the attestations light client requires the proven key path to be exactly one
// element). recvPacket reads this back: counterparty.ClientId must equal the packet's SourceClient, and
// counterparty.MerklePrefix is prepended to the commitment key the attestation signs over.
func (c *Client) RegisterCounterparty(ctx context.Context, destClient string) (txHash string, err error) {
	msg := clientv2types.NewMsgRegisterCounterparty(
		destClient, counterpartyMerklePrefix(), evmCounterpartyClientID, c.signer.address,
	)
	hash, _, err := c.submitMsgs(ctx, []sdk.Msg{msg})
	if err != nil {
		return "", fmt.Errorf("cosmos: register counterparty: %w", err)
	}
	return hash, nil
}

// FundICS27 funds the ICS-27 executor account from the escrow with the GMP counter denom (for increments)
// and returns the tx hash. Without this an increment's inner MsgSend would fail insufficient-funds; funding
// it at deploy makes that a loud, one-time setup step. The receiving bank send creates the account, so no
// separate staking-denom leg is needed.
func (c *Client) FundICS27(ctx context.Context, ics27Addr string) (txHash string, err error) {
	coins := sdk.NewCoins(sdk.NewCoin(GMPDenom, sdkmath.NewInt(ics27FundGMP)))
	msg := &banktypes.MsgSend{FromAddress: c.signer.address, ToAddress: ics27Addr, Amount: coins}
	hash, _, err := c.submitMsgs(ctx, []sdk.Msg{msg})
	return hash, err
}

// DeliverGMP delivers one evm->cosmos GMP packet to the real ICS-27 module via a signed MsgRecvPacket, proven
// by the attestations light client, and returns the recv tx hash, the acknowledgement outcome, and (on
// failure) a truthful reason extracted from the module's error event. The inner effect is a CosmosTx the
// module runs as the ICS-27 account:
//
//   - increment payload  -> one bank MsgSend of 1 <gmpDenom> ICS27->target: the +1 counter-denom delta at the
//     target is the exactly-once increment (success ack).
//   - any other payload  -> a bank MsgSend of an amount exceeding the ICS-27 balance: the module's atomic
//     execution fails insufficient-funds and returns the universal error acknowledgement, leaving the target
//     unchanged (the cached context is discarded).
//
// It is idempotent on (destClient, seq): before submitting it looks for an already-written acknowledgement for
// this packet (the module's replay protection would otherwise no-op the recv without re-emitting an ack), and
// if found returns that recorded outcome without re-sending.
func (c *Client) DeliverGMP(
	ctx context.Context,
	destClient, ics27Addr, gmpDenom, target string,
	payload []byte,
	seq uint64,
) (txHash string, success bool, reason string, err error) {
	// Idempotency: a committed acknowledgement for this packet means it was already delivered. Re-submitting
	// would hit the module's packet-receipt replay guard (a no-op that emits no fresh ack), so recover the
	// recorded outcome from the original recv instead of re-sending.
	if h, events, ok, ferr := c.findAckTx(ctx, destClient, seq); ferr != nil {
		return "", false, "", ferr
	} else if ok {
		return classifyAck(h, events, destClient, seq)
	}

	packet, err := c.buildGMPPacket(destClient, ics27Addr, gmpDenom, target, payload, seq)
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
		return "", false, "", fmt.Errorf("cosmos: submit MsgRecvPacket (seq %d): %w", seq, err)
	}
	return classifyAck(hash, events, destClient, seq)
}

// classifyAck reads the acknowledgement outcome for (destClient, seq) out of a recv tx's events: the recv tx
// commits with code 0 whether the ICS-27 execution succeeded or produced an error ack, so success/failure is
// determined from the write_acknowledgement bytes, and (on failure) a truthful reason from the module's error
// event. It is the shared tail of both the fresh-delivery and idempotent-replay paths.
func classifyAck(hash string, events []abci.Event, destClient string, seq uint64) (string, bool, string, error) {
	ok, success, err := ackSuccessFromEvents(events, destClient, seq)
	if err != nil {
		return "", false, "", err
	}
	if !ok {
		return "", false, "", fmt.Errorf("cosmos: recv tx %s wrote no acknowledgement for seq %d", hash, seq)
	}
	if success {
		return hash, true, "", nil
	}
	return hash, false, gmpErrorReason(events), nil
}

// buildGMPPacket assembles the IBC v2 packet the module will receive: a GMPPacketData whose Payload is the
// serialized CosmosTx to execute (increment or a deliberately-failing send), wrapped in the gmp port/version,
// carried on a packet from the fabricated source client to the destination attestations client at the
// source-provided sequence, with a future timeout.
func (c *Client) buildGMPPacket(
	destClient, ics27Addr, gmpDenom, target string,
	payload []byte,
	seq uint64,
) (channelv2.Packet, error) {
	inner, err := innerCosmosTx(ics27Addr, gmpDenom, target, payload)
	if err != nil {
		return channelv2.Packet{}, err
	}
	gpd := gmptypes.NewGMPPacketData(GMPSender, target, nil, inner, "")
	value, err := gmptypes.MarshalPacketData(&gpd, gmptypes.Version, gmptypes.EncodingProtobuf)
	if err != nil {
		return channelv2.Packet{}, fmt.Errorf("cosmos: marshal GMP packet data: %w", err)
	}
	payloadV2 := channelv2.NewPayload(
		gmptypes.PortID,
		gmptypes.PortID,
		gmptypes.Version,
		gmptypes.EncodingProtobuf,
		value,
	)
	timeout := uint64(time.Now().Add(packetTimeoutWindow).Unix())
	return channelv2.NewPacket(seq, evmCounterpartyClientID, destClient, timeout, payloadV2), nil
}

// innerCosmosTx builds the ICS-27 payload: a CosmosTx of a single bank MsgSend SIGNED BY (from) the ICS-27
// account (the module authenticates that every inner msg's sole signer is that account). The increment marker
// sends 1 <gmpDenom> to the target (the real +1); any other payload sends an over-balance amount so the
// module's atomic execution fails and yields an error acknowledgement.
func innerCosmosTx(ics27Addr, gmpDenom, target string, payload []byte) ([]byte, error) {
	amount := ics27ErrorAckAmount
	if isIncrement(payload) {
		amount = "1"
	}
	amt, ok := sdkmath.NewIntFromString(amount)
	if !ok {
		return nil, fmt.Errorf("cosmos: bad inner amount %q", amount)
	}
	msg := &banktypes.MsgSend{
		FromAddress: ics27Addr,
		ToAddress:   target,
		Amount:      sdk.NewCoins(sdk.NewCoin(gmpDenom, amt)),
	}
	bz, err := gmptypes.SerializeCosmosTx(cdc, []gogoproto.Message{msg})
	if err != nil {
		return nil, fmt.Errorf("cosmos: serialize inner CosmosTx: %w", err)
	}
	return bz, nil
}

// isIncrement reports whether the delivered GMP payload is the harness's opaque "increment" marker. The
// payload semantics are family-specific: the harness passes an opaque marker across its family-agnostic
// surface and the stub — which owns the relayer's payload constructor — maps it to the real increment
// CosmosTx. The real ibc link binary would accept a real serialized CosmosTx here; this seam keeps the
// harness from having to build ibc-go protos.
func isIncrement(payload []byte) bool { return string(payload) == GMPIncrementPayload }

// buildAttestationProof forges the 1-of-1 packet-commitment attestation the destination light client verifies:
// it computes the exact membership (path, value) the channel keeper will check — value = CommitPacket(packet),
// path = keccak256 over the counterparty-prefixed packet-commitment key — ABI-encodes a PacketAttestation at
// the nominal client height, signs the domain-tagged digest with the attestor key (go-ethereum's crypto.Sign
// already yields v in {0,1}, which the light client accepts as-is), and returns the gogoproto-marshaled
// AttestationProof used as MsgRecvPacket.ProofCommitment.
func buildAttestationProof(packet channelv2.Packet) ([]byte, error) {
	key := hostv2.PacketCommitmentKey(packet.SourceClient, packet.Sequence)
	return buildMembershipAttestation(key, channelv2.CommitPacket(packet))
}

func buildMembershipAttestation(key, value []byte) ([]byte, error) {
	merklePath := channelv2.BuildMerklePath(counterpartyMerklePrefix(), key)
	if len(merklePath.KeyPath) != 1 {
		return nil, fmt.Errorf(
			"cosmos: attestations client needs a single-element key path, got %d",
			len(merklePath.KeyPath),
		)
	}
	attPath := crypto.Keccak256(merklePath.KeyPath[0])

	pa := attestations.PacketAttestation{
		Height:  attestationsClientHeight,
		Packets: []attestations.PacketCompact{{Path: attPath, Commitment: value}},
	}
	attData, err := pa.ABIEncode()
	if err != nil {
		return nil, fmt.Errorf("cosmos: ABI-encode packet attestation: %w", err)
	}
	digest := attestations.TaggedSigningInput(attData, attestations.AttestationTypePacket)
	sig, err := crypto.Sign(digest[:], attestorKey)
	if err != nil {
		return nil, fmt.Errorf("cosmos: sign attestation: %w", err)
	}
	proofBz, err := cdc.Marshal(&attestations.AttestationProof{
		AttestationData: attData,
		Signatures:      [][]byte{sig},
	})
	if err != nil {
		return nil, fmt.Errorf("cosmos: marshal attestation proof: %w", err)
	}
	return proofBz, nil
}

// counterpartyMerklePrefix is the single-element ICS24 prefix registered for the counterparty and prepended to
// the packet-commitment key. A single element is required: the attestations light client verifies a key path
// of exactly length one. The concrete bytes are arbitrary as long as the counterparty registration and the
// attestation agree (both derive from this one function), so the standard "ibc" prefix is used.
func counterpartyMerklePrefix() [][]byte { return [][]byte{[]byte("ibc")} }

// submitMsgs signs (as the escrow), broadcasts, and waits for commit of a tx carrying msgs at the module gas
// budget, then returns the committed tx hash and its result events. A non-zero result code fails in waitTx; a
// committed tx that wrote an error acknowledgement still has code 0 (the recv itself succeeded), so the caller
// inspects the events to classify success vs error-ack.
func (c *Client) submitMsgs(ctx context.Context, msgs []sdk.Msg) (string, []abci.Event, error) {
	accountNumber, sequence, err := c.account(ctx)
	if err != nil {
		return "", nil, err
	}
	txBytes, err := c.signMsgs(msgs, gmpGasLimit, accountNumber, sequence)
	if err != nil {
		return "", nil, err
	}
	hash, err := c.broadcast(ctx, txBytes)
	if err != nil {
		return "", nil, err
	}
	if waitErr := c.waitTx(ctx, hash); waitErr != nil {
		return "", nil, waitErr
	}
	events, err := c.txEvents(ctx, hash)
	if err != nil {
		return "", nil, err
	}
	return hash, events, nil
}

// findAckTx searches the tx index for an already-committed write_acknowledgement for (destClient, seq) and
// returns its tx hash and result events — the idempotency guard DeliverGMP consults before submitting. The
// caller classifies the outcome from the returned events (the same events the fresh path inspects).
func (c *Client) findAckTx(
	ctx context.Context,
	destClient string,
	seq uint64,
) (hash string, events []abci.Event, found bool, err error) {
	query := fmt.Sprintf("%s.%s='%s'", channelv2.EventTypeWriteAck, channelv2.AttributeKeyDstClient, destClient)
	err = c.forEachTx(ctx, query, func(tx *coretypes.ResultTx) (bool, error) {
		if tx.TxResult.Code != 0 {
			return false, nil
		}
		ok, _, derr := ackSuccessFromEvents(tx.TxResult.Events, destClient, seq)
		if derr != nil {
			return false, derr
		}
		if ok {
			hash = tx.Hash.String()
			events = tx.TxResult.Events
			found = true
			return true, nil
		}
		return false, nil
	})
	return hash, events, found, err
}

// ackSuccessFromEvents finds the write_acknowledgement event for (destClient, seq) among a tx's events and
// decodes its acknowledgement, returning whether it was found and whether it was a success (vs the universal
// error acknowledgement). Success is the canonical channel-level signal, decoded from the real ack bytes.
func ackSuccessFromEvents(events []abci.Event, destClient string, seq uint64) (found bool, success bool, err error) {
	seqStr := fmt.Sprintf("%d", seq)
	for _, ev := range events {
		if ev.Type != channelv2.EventTypeWriteAck {
			continue
		}
		attrs := attrMap(ev)
		if attrs[channelv2.AttributeKeyDstClient] != destClient || attrs[channelv2.AttributeKeySequence] != seqStr {
			continue
		}
		encoded := attrs[channelv2.AttributeKeyEncodedAckHex]
		raw, derr := hex.DecodeString(encoded)
		if derr != nil {
			return false, false, fmt.Errorf("cosmos: decode ack hex: %w", derr)
		}
		var ack channelv2.Acknowledgement
		if uerr := ack.Unmarshal(raw); uerr != nil {
			return false, false, fmt.Errorf("cosmos: unmarshal acknowledgement: %w", uerr)
		}
		return true, ack.Success(), nil
	}
	return false, false, nil
}

// gmpErrorReason extracts the module's execution error from the recv tx events for an error acknowledgement.
// On a failed OnRecvPacket the channel keeper prefixes the app's emitted event type and attribute keys with
// "ibccallbackerror-", so the gmp module's recv event carries the real error under a prefixed key. A generic
// reason is returned when no such attribute is present (still truthful: the ack was the universal error ack).
func gmpErrorReason(events []abci.Event) string {
	const errPrefix = "ibccallbackerror-"
	for _, ev := range events {
		if strings.TrimPrefix(ev.Type, errPrefix) != gmptypes.EventTypeRecvPacket {
			continue
		}
		for _, a := range ev.Attributes {
			if strings.TrimPrefix(a.Key, errPrefix) == gmptypes.AttributeKeyAckError && a.Value != "" {
				return a.Value
			}
		}
	}
	return "GMP module returned an error acknowledgement (ICS-27 inner execution failed)"
}

// clientIDFromEvents reads the created client id from a create_client event.
func clientIDFromEvents(events []abci.Event) (string, bool) {
	for _, ev := range events {
		if ev.Type != clienttypes.EventTypeCreateClient {
			continue
		}
		for _, a := range ev.Attributes {
			if a.Key == clienttypes.AttributeKeyClientID {
				return a.Value, true
			}
		}
	}
	return "", false
}

// attrMap flattens an event's attributes into a lookup map.
func attrMap(ev abci.Event) map[string]string {
	m := make(map[string]string, len(ev.Attributes))
	for _, a := range ev.Attributes {
		m[a.Key] = a.Value
	}
	return m
}
