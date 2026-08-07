// Package proofgen generates packet membership/non-membership proofs and
// light-client state proofs. There is one implementation per light-client
// type.
package proofgen

import (
	"context"
	"log/slog"
	"strings"
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

	// FinalityOffset is the finality offset resolved for this light client at
	// construction time.
	FinalityOffset() uint64
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
// service when running in dual mode (nil otherwise). Resolution -- querying
// each client end's on-chain attestation set and matching it against the
// top-level attestors[] list -- happens once, here, at construction time; the
// resulting ProofGenerator's runtime methods never re-query on-chain state or
// re-probe attestor endpoints.
func NewSetFromConfig(
	ctx context.Context,
	cfg config.Config,
	clientSet *chains.ClientSet,
	localAttestors *attestor.Service,
) (*Set, error) {
	candidates := resolveConfiguredAttestors(ctx, cfg.Attestors, localAttestors)

	generators := make(map[string]ProofGenerator, len(cfg.Relayer.Connections)*2)

	for _, conn := range cfg.Relayer.Connections {
		for _, end := range []struct {
			self, counterparty config.ClientEnd
		}{
			{conn.ClientA, conn.ClientB},
			{conn.ClientB, conn.ClientA},
		} {
			if err := addGenerator(
				ctx,
				generators,
				conn.Alias,
				end.self,
				end.counterparty,
				clientSet,
				candidates,
			); err != nil {
				return nil, err
			}
		}
	}

	return NewSet(generators), nil
}

func addGenerator(
	ctx context.Context,
	generators map[string]ProofGenerator,
	connAlias string,
	self, counterparty config.ClientEnd,
	clientSet *chains.ClientSet,
	candidates []attestor.Attestor,
) error {
	switch self.Type {
	case config.ClientTypeAttestation:
		selfChain, ok := clientSet.Get(self.ChainID)
		if !ok {
			return errors.Errorf("connection %q: no configured chain client for %q", connAlias, self.ChainID)
		}

		onChainAddrs, minRequiredSigs, err := selfChain.GetAttestationSet(ctx, self.ClientID)
		if err != nil {
			return errors.Wrapf(
				err, "connection %q: querying on-chain attestation set for client %q", connAlias, self.ClientID,
			)
		}

		onChainSet := make(map[string]struct{}, len(onChainAddrs))
		for _, addr := range onChainAddrs {
			onChainSet[strings.ToLower(addr)] = struct{}{}
		}

		var matched []attestor.Attestor

		var finalityOffset uint64

		for _, c := range candidates {
			if c.ChainID() != counterparty.ChainID {
				continue
			}

			if _, inOnChainSet := onChainSet[strings.ToLower(c.Address())]; !inOnChainSet {
				continue
			}

			matched = append(matched, c)

			if offset := c.FinalityOffset(); offset > finalityOffset {
				finalityOffset = offset
			}
		}

		if len(matched) < int(minRequiredSigs) {
			return errors.Errorf(
				"connection %q: only %d reachable/matching attestors for chain %q, on-chain quorum requires %d",
				connAlias, len(matched), counterparty.ChainID, minRequiredSigs,
			)
		}

		counterpartyChain, ok := clientSet.Get(counterparty.ChainID)
		if !ok {
			return errors.Errorf(
				"connection %q: no configured chain client for counterparty chain %q",
				connAlias, counterparty.ChainID,
			)
		}

		generators[Key(self.ChainID, self.ClientID)] = attestation.New(
			matched, int(minRequiredSigs), finalityOffset, counterpartyChain,
		)
	default:
		return errors.Errorf("connection %q: unsupported client type %q for proof generation", connAlias, self.Type)
	}

	return nil
}

// resolveConfiguredAttestors resolves every top-level attestors[] entry to
// its live identity (chain, address, finality offset) once, up front --
// which client ends they end up authorized for is decided per-connection in
// addGenerator by matching against on-chain state. An entry that can't be
// resolved right now (unreachable endpoint, misconfigured local wiring) is
// logged and excluded rather than failing the whole set: it simply won't
// count toward any client end's quorum, the same as if it weren't configured
// at all.
func resolveConfiguredAttestors(
	ctx context.Context,
	entries config.Attestors,
	localAttestors *attestor.Service,
) []attestor.Attestor {
	resolved := make([]attestor.Attestor, 0, len(entries))

	for _, entry := range entries {
		a, err := resolveAttestor(ctx, entry, localAttestors)
		if err != nil {
			slog.Warn("Skipping unresolvable configured attestor", "name", entry.Name, "type", entry.Type, "err", err)
			continue
		}

		resolved = append(resolved, a)
	}

	return resolved
}

// resolveAttestor resolves one configured attestor entry to the
// attestor.Attestor abstraction, live: local entries via the already-running
// Service, remote entries via a one-off Info RPC call.
func resolveAttestor(
	ctx context.Context,
	entry config.AttestorConfig,
	localAttestors *attestor.Service,
) (attestor.Attestor, error) {
	switch entry.Type {
	case config.AttestorTypeRemote:
		if entry.GRPC == "" {
			return nil, errors.New("no grpc address configured")
		}

		// TODO: Support TLS
		grpcURL := "http://" + entry.GRPC

		chainID, address, finalityOffset, err := attestor.QueryInfo(ctx, grpcURL, entry.Name)
		if err != nil {
			return nil, errors.Wrap(err, "querying attestor info")
		}

		return attestor.NewRemoteFromURL(chainID, entry.Name, address, finalityOffset, grpcURL), nil
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
