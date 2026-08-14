// SPDX-License-Identifier: Apache-2.0

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

// NewSetFromConfig resolves a ProofGenerator for every client end of every
// configured connection, matching against attestors (this process's own
// local attestors plus every resolved remote one).
func NewSetFromConfig(
	ctx context.Context,
	cfg config.Config,
	clientSet *chains.ClientSet,
	attestors []attestor.Attestor,
) (*Set, error) {
	generators := make(map[string]ProofGenerator, len(cfg.Relayer.Connections)*2)

	err := forEachClientEnd(cfg, func(connAlias string, self, counterparty config.ClientEnd) error {
		return addGenerator(ctx, generators, connAlias, self, counterparty, clientSet, attestors)
	})
	if err != nil {
		return nil, err
	}

	return NewSet(generators), nil
}

// forEachClientEnd calls fn once per client end of every configured
// connection, in both directions.
func forEachClientEnd(cfg config.Config, fn func(connAlias string, self, counterparty config.ClientEnd) error) error {
	for _, conn := range cfg.Relayer.Connections {
		for _, end := range []struct {
			self, counterparty config.ClientEnd
		}{
			{conn.ClientA, conn.ClientB},
			{conn.ClientB, conn.ClientA},
		} {
			if err := fn(conn.Alias, end.self, end.counterparty); err != nil {
				return err
			}
		}
	}

	return nil
}

func addGenerator(
	ctx context.Context,
	generators map[string]ProofGenerator,
	connAlias string,
	client, clientCounterparty config.ClientEnd,
	clientSet *chains.ClientSet,
	attestors []attestor.Attestor,
) error {
	switch client.Type {
	case config.ClientTypeAttestation:
		gen, err := attestation.ResolveGenerator(ctx, client, clientCounterparty, clientSet, attestors)
		if err != nil {
			return err
		}

		generators[Key(client.ChainID, client.ClientID)] = gen

		return nil
	default:
		return errors.Errorf("connection %q: unsupported client type %q for proof generation", connAlias, client.Type)
	}
}
