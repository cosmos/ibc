package attestation

import (
	"context"
	"time"

	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/proofgen"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	attestorevm "github.com/cosmos/ibc/link/internal/service/attestor/evm"
	"github.com/cosmos/ibc/link/internal/service/signer"
)

// Generator implements proofgen.ProofGenerator for one configured
// attestation light client: LatestProvableHeight/StateProof/PacketProofs all
// query the same fixed attestor set and threshold; LatestProvableHeight
// additionally reads the counterparty chain directly for the resolved
// height's timestamp.
type Generator struct {
	attestors         []attestor.Attestor
	threshold         int
	counterpartyChain chains.Client
}

var _ proofgen.ProofGenerator = (*Generator)(nil)

func New(attestors []attestor.Attestor, threshold int, counterpartyChain chains.Client) *Generator {
	return &Generator{attestors: attestors, threshold: threshold, counterpartyChain: counterpartyChain}
}

func (g *Generator) LatestProvableHeight(ctx context.Context) (uint64, time.Time, error) {
	return latestProvableHeight(ctx, g.attestors, g.threshold, g.counterpartyChain)
}

func (g *Generator) StateProof(ctx context.Context, height uint64) ([]byte, error) {
	result, err := queryStateQuorum(ctx, g.attestors, g.threshold, height)
	if err != nil {
		return nil, errors.Wrap(err, "querying state attestation quorum")
	}

	decodedHeight, _, err := attestorevm.DecodeStateAttestation(result.AttestationData)
	if err != nil {
		return nil, errors.Wrap(err, "decoding state attestation quorum result")
	}

	if decodedHeight != height {
		return nil, errors.Errorf("state attestation height %d does not match requested height %d", decodedHeight, height)
	}

	proof, err := encodeAttestationProof(AttestationProof{
		AttestationData: result.AttestationData,
		Signatures:      result.Signatures,
	})
	if err != nil {
		return nil, errors.Wrap(err, "encoding state attestation proof")
	}

	return proof, nil
}

func (g *Generator) PacketProofs(
	ctx context.Context,
	height uint64,
	kind proofgen.ProofKind,
	packets []channeltypesv2.Packet,
) ([][]byte, error) {
	commitmentType, err := commitmentTypeOf(kind)
	if err != nil {
		return nil, err
	}

	encodedPackets := make([][]byte, len(packets))

	for i, packet := range packets {
		encoded, errEnc := attestorevm.EncodePacket(packet)
		if errEnc != nil {
			return nil, errors.Wrapf(errEnc, "encoding packet sequence %d", packet.Sequence)
		}

		encodedPackets[i] = encoded
	}

	result, err := queryPacketQuorum(ctx, g.attestors, g.threshold, encodedPackets, height, commitmentType)
	if err != nil {
		return nil, errors.Wrap(err, "querying packet attestation quorum")
	}

	decodedHeight, decodedPackets, err := attestorevm.DecodePacketAttestation(result.AttestationData)
	if err != nil {
		return nil, errors.Wrap(err, "decoding packet attestation quorum result")
	}

	if decodedHeight != height {
		return nil, errors.Errorf("packet attestation height %d does not match requested height %d", decodedHeight, height)
	}

	if len(decodedPackets) != len(packets) {
		return nil, errors.Errorf("packet attestation returned %d packets, expected %d", len(decodedPackets), len(packets))
	}

	proof, err := encodeAttestationProof(AttestationProof{
		AttestationData: result.AttestationData,
		Signatures:      result.Signatures,
	})
	if err != nil {
		return nil, errors.Wrap(err, "encoding packet attestation proof")
	}

	// The attestor returns one shared proof blob covering every packet in the
	// batch; PacketProofs' contract is one proof per input packet, so the same
	// blob is returned len(packets) times rather than re-querying per packet.
	proofs := make([][]byte, len(packets))
	for i := range proofs {
		proofs[i] = proof
	}

	return proofs, nil
}

func commitmentTypeOf(kind proofgen.ProofKind) (attestor.CommitmentType, error) {
	switch kind {
	case proofgen.KindPacketCommitment:
		return attestor.CommitmentTypePacket, nil
	case proofgen.KindAcknowledgement:
		return attestor.CommitmentTypeAck, nil
	case proofgen.KindReceiptAbsence:
		return attestor.CommitmentTypeReceipt, nil
	default:
		return 0, errors.Errorf("unsupported proof kind %v", kind)
	}
}

// NewSetFromConfig resolves every configured client's attestor set, up
// front, so the request path only ever queries already-resolved attestors.
// clientSet resolves the chain client for each client's counterparty chain,
// used to look up a resolved height's timestamp directly.
func NewSetFromConfig(
	cfg config.Config,
	clientSet *chains.ClientSet,
	signers *signer.Set,
) (*proofgen.Set, error) {
	generators := make(map[string]proofgen.ProofGenerator, len(cfg.Relayer.Clients))

	for _, clientCfg := range cfg.Relayer.Clients {
		if clientCfg.AttestorSet == nil {
			continue
		}

		attestors := make([]attestor.Attestor, 0, len(clientCfg.AttestorSet.Attestors))

		for _, entry := range clientCfg.AttestorSet.Attestors {
			a, err := resolveAttestor(cfg, entry, clientCfg.CounterpartyChainID, clientSet, signers)
			if err != nil {
				return nil, errors.Wrapf(err, "client %q attestor %q", clientCfg.Alias, entry.Name)
			}

			attestors = append(attestors, a)
		}

		counterpartyChain, ok := clientSet.Get(clientCfg.CounterpartyChainID)
		if !ok {
			return nil, errors.Errorf(
				"client %q: no configured chain client for counterparty chain %q", clientCfg.Alias, clientCfg.CounterpartyChainID,
			)
		}

		generators[proofgen.Key(clientCfg.ChainID, clientCfg.ClientID)] = New(attestors, clientCfg.AttestorSet.Threshold, counterpartyChain)
	}

	return proofgen.NewSet(generators), nil
}

// resolveAttestor resolves one configured attestor-set entry to the
// attestor.Attestor abstraction: a remote entry dials it; a local entry
// constructs the same attestor.LocalAttestor implementation this process
// would run in dual mode, in-process, with no client or dial involved.
// watchedChainID is the chain the attestor is expected to be attesting,
// used only to label the remote attestor for logging.
func resolveAttestor(
	cfg config.Config,
	entry config.AttestorEntry,
	watchedChainID string,
	clientSet *chains.ClientSet,
	signers *signer.Set,
) (attestor.Attestor, error) {
	switch entry.Type {
	case config.AttestorTypeRemote:
		if entry.GRPC == "" {
			return nil, errors.New("no grpc address configured")
		}

		return attestor.NewRemoteFromURL(watchedChainID, entry.Name, entry.Name, "http://"+entry.GRPC), nil
	case config.AttestorTypeLocal:
		spec, ok := findAttestationConfig(cfg, entry.Name)
		if !ok {
			return nil, errors.Errorf("no local attestor configuration found for %q", entry.Name)
		}

		client, ok := clientSet.Get(spec.ChainID)
		if !ok {
			return nil, errors.Errorf("no configured chain client for chain %q", spec.ChainID)
		}

		s, ok := signers.Get(spec.Signer)
		if !ok {
			return nil, errors.Errorf("unknown signer %q", spec.Signer)
		}

		return attestor.NewLocal(spec, client, s)
	default:
		return nil, errors.Errorf("type %q not yet supported for proof generation", entry.Type)
	}
}

func findAttestationConfig(cfg config.Config, name string) (config.AttestationConfig, bool) {
	for _, spec := range cfg.Attestor.Attestations {
		if spec.Name == name {
			return spec, true
		}
	}

	return config.AttestationConfig{}, false
}
