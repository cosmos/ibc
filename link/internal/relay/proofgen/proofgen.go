// Package proofgen generates packet membership/non-membership proofs and
// light-client state proofs. There is one implementation per light-client
// type.
package proofgen

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/proofgen/attestation"
	"github.com/cosmos/ibc/link/internal/service/attestor"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
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

// NewSetFromConfig resolves a ProofGenerator for every client end of every
// configured connection. localAttestors is this process's own attestor
// service when running in dual mode (nil otherwise)
func NewSetFromConfig(
	cfg config.Config,
	clientSet *chains.ClientSet,
	localAttestors *attestor.Service,
) (*Set, error) {
	generators := make(map[string]ProofGenerator, len(cfg.Relayer.Connections)*2)

	for _, conn := range cfg.Relayer.Connections {
		for _, end := range []struct {
			self, counterparty config.ClientEndConfig
		}{
			{conn.ClientA, conn.ClientB},
			{conn.ClientB, conn.ClientA},
		} {
			if err := addGenerator(generators, cfg, conn.Alias, end.self, end.counterparty, clientSet, localAttestors); err != nil {
				return nil, err
			}
		}
	}

	return NewSet(generators), nil
}

func addGenerator(
	generators map[string]ProofGenerator,
	cfg config.Config,
	connAlias string,
	self, counterparty config.ClientEndConfig,
	clientSet *chains.ClientSet,
	localAttestors *attestor.Service,
) error {
	switch self.Type {
	case config.ClientTypeAttestation:
		if self.AttestorSet == nil {
			return errors.Errorf("connection %q: attestorSet required for attestation clients", connAlias)
		}

		attestors := make([]attestor.Attestor, 0, len(self.AttestorSet.Attestors))

		for _, alias := range self.AttestorSet.Attestors {
			entry, ok := cfg.Attestors.ByAlias(alias)
			if !ok {
				return errors.Errorf("connection %q: attestor %q not declared in top-level attestors", connAlias, alias)
			}

			a, err := resolveAttestor(entry, counterparty.ChainID, localAttestors)
			if err != nil {
				return errors.Wrapf(err, "connection %q attestor %q", connAlias, alias)
			}

			attestors = append(attestors, a)
		}

		counterpartyChain, ok := clientSet.Get(counterparty.ChainID)
		if !ok {
			return errors.Errorf(
				"connection %q: no configured chain client for counterparty chain %q",
				connAlias, counterparty.ChainID,
			)
		}

		generators[Key(self.ChainID, self.ClientID)] = attestation.New(
			attestors,
			self.AttestorSet.Threshold,
			counterpartyChain,
		)
	default:
		return errors.Errorf("connection %q: unsupported client type %q for proof generation", connAlias, self.Type)
	}

	return nil
}

// resolveAttestor resolves one configured attestor alias to the
// attestor.Attestor abstraction. watchedChainID is the sibling ClientEnd's
// chain ID within the same connection -- the chain this attestor is expected
// to be attesting.
func resolveAttestor(
	entry config.AttestorConfig,
	watchedChainID string,
	localAttestors *attestor.Service,
) (attestor.Attestor, error) {
	switch entry.Type {
	case config.AttestorTypeRemote:
		if entry.GRPC == "" {
			return nil, errors.New("no grpc address configured")
		}

		// TODO: Support TLS
		return attestor.NewRemoteFromURL(watchedChainID, entry.Name, entry.Alias, "http://"+entry.GRPC), nil
	case config.AttestorTypeLocal:
		if localAttestors == nil {
			return nil, errors.Errorf("no local attestor configuration found for %q", entry.Alias)
		}

		a, ok := localAttestors.Get(entry.Alias)
		if !ok {
			return nil, errors.Errorf("no local attestor configuration found for %q", entry.Alias)
		}

		return a, nil
	default:
		return nil, errors.Errorf("type %q not yet supported for proof generation", entry.Type)
	}
}
