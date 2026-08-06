// Package proofgen generates packet membership/non-membership proofs and
// light-client state proofs. There is one implementation per light-client
// type.
package proofgen

import (
	"context"
	"time"

	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/proofgen/attestation"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// ProofGenerator generates packet membership/non-membership proofs and state
// proofs for one configured light client.
type ProofGenerator interface {
	// LatestProvableHeight resolves the highest height a subsequent StateProof
	// and PacketProofs call sharing that height can currently succeed at,
	// along with that height's counterparty-chain timestamp
	LatestProvableHeight(ctx context.Context) (uint64, time.Time, error)

	// StateProof proves the light client's counterparty state at height.
	StateProof(ctx context.Context, height uint64) ([]byte, error)

	// PacketProofs proves each packet's membership or non-membership at
	// height, one proof per packet with indices aligned to packets. Returns
	// an error if a proof cannot be generated for any packet
	PacketProofs(
		ctx context.Context,
		height uint64,
		kind v2.ProofKind,
		packets []channeltypesv2.Packet,
	) ([][]byte, error)
}

var _ ProofGenerator = (*attestation.Generator)(nil)

// Key identifies one configured light client by the chain it lives on and
// its client id, the composite key ProofGenerator instances are scoped by.
func Key(chainID, clientID string) string {
	return chainID + "/" + clientID
}

// Set resolves a ProofGenerator by (chainID, clientID).
type Set struct {
	generators map[string]ProofGenerator
}

func NewSet(generators map[string]ProofGenerator) *Set {
	if generators == nil {
		generators = make(map[string]ProofGenerator)
	}

	return &Set{generators: generators}
}

func (s *Set) Get(chainID, clientID string) (ProofGenerator, bool) {
	generator, ok := s.generators[Key(chainID, clientID)]
	return generator, ok
}

// NewSetFromConfig resolves every configured client's attestor set. localAttestors
// is this process's own attestor service when running in dual mode (nil
// otherwise)
func NewSetFromConfig(
	cfg config.Config,
	clientSet *chains.ClientSet,
	localAttestors *attestor.Service,
) (*Set, error) {
	generators := make(map[string]ProofGenerator, len(cfg.Relayer.Clients))

	for _, clientCfg := range cfg.Relayer.Clients {
		switch clientCfg.Type {
		case config.ClientTypeAttestation:
			if clientCfg.AttestorSet == nil {
				return nil, errors.Errorf("client %q: attestorSet required for attestation clients", clientCfg.Alias)
			}

			attestors := make([]attestor.Attestor, 0, len(clientCfg.AttestorSet.Attestors))

			for _, entry := range clientCfg.AttestorSet.Attestors {
				a, err := resolveAttestor(entry, clientCfg.CounterpartyChainID, localAttestors)
				if err != nil {
					return nil, errors.Wrapf(err, "client %q attestor %q", clientCfg.Alias, entry.Name)
				}

				attestors = append(attestors, a)
			}

			counterpartyChain, ok := clientSet.Get(clientCfg.CounterpartyChainID)
			if !ok {
				return nil, errors.Errorf(
					"client %q: no configured chain client for counterparty chain %q",
					clientCfg.Alias,
					clientCfg.CounterpartyChainID,
				)
			}

			generators[Key(clientCfg.ChainID, clientCfg.ClientID)] = attestation.New(
				attestors,
				clientCfg.AttestorSet.Threshold,
				counterpartyChain,
			)
		default:
			return nil, errors.Errorf(
				"client %q: unsupported client type %q for proof generation",
				clientCfg.Alias,
				clientCfg.Type,
			)
		}
	}

	return NewSet(generators), nil
}

// resolveAttestor resolves one configured attestor-set entry to the
// attestor.Attestor abstraction. watchedChainID is the chain
// the attestor is expected to be attesting
func resolveAttestor(
	entry config.AttestorEntry,
	watchedChainID string,
	localAttestors *attestor.Service,
) (attestor.Attestor, error) {
	switch entry.Type {
	case config.AttestorTypeRemote:
		if entry.GRPC == "" {
			return nil, errors.New("no grpc address configured")
		}

		// TODO: Support TLS
		return attestor.NewRemoteFromURL(watchedChainID, entry.Name, entry.Name, "http://"+entry.GRPC), nil
	case config.AttestorTypeLocal:
		if localAttestors == nil {
			return nil, errors.Errorf("no local attestor configuration found for %q", entry.Name)
		}

		a, ok := localAttestors.Get(entry.Name)
		if !ok {
			return nil, errors.Errorf("no local attestor configuration found for %q", entry.Name)
		}

		return a, nil
	default:
		return nil, errors.Errorf("type %q not yet supported for proof generation", entry.Type)
	}
}
