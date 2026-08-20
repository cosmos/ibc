// SPDX-License-Identifier: Apache-2.0

// Package prover generates packet membership/non-membership proofs and
// light-client state proofs. There is one implementation per light-client
// type.
package prover

import (
	"context"
	"time"

	"github.com/pkg/errors"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/prover/attestation"
	"github.com/cosmos/ibc/link/internal/relay/prover/remote"
	"github.com/cosmos/ibc/link/internal/service/attestor"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

// Prover generates packet membership/non-membership proofs and state
// proofs for one configured light client.
type Prover interface {
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

var _ Prover = (*attestation.Generator)(nil)

// Key identifies one configured light client by the chain it lives on and
// its client id, the composite key Prover instances are scoped by.
func Key(chainID, clientID string) string {
	return chainID + "/" + clientID
}

// Set resolves a Prover by (chainID, clientID).
type Set struct {
	generators map[string]Prover
}

func NewSet(generators map[string]Prover) *Set {
	if generators == nil {
		generators = make(map[string]Prover)
	}

	return &Set{generators: generators}
}

func (s *Set) Get(chainID, clientID string) (Prover, bool) {
	generator, ok := s.generators[Key(chainID, clientID)]
	return generator, ok
}

// NewSetFromConfig resolves a Prover for every client end of every
// configured connection, matching against attestors (this process's own
// local attestors plus every resolved remote one).
func NewSetFromConfig(
	ctx context.Context,
	cfg config.Config,
	clientSet *chains.ClientSet,
	attestors []attestor.Attestor,
) (*Set, error) {
	generators := make(map[string]Prover, len(cfg.Relayer.Connections)*2)

	err := forEachClientEnd(cfg, func(connAlias string, self, counterparty config.ClientEnd) error {
		return addGenerator(ctx, generators, connAlias, self, counterparty, clientSet, attestors)
	})
	if err != nil {
		return nil, err
	}

	return NewSet(generators), nil
}

// NewAttestationSetFromConfig resolves an attestation prover for every client
// end regardless of its configured type. This is the set a relayer serves over
// ProverService: resolving a remoteProver client here would dial back out and
// recurse, so the served set is always local.
func NewAttestationSetFromConfig(
	ctx context.Context,
	cfg config.Config,
	clientSet *chains.ClientSet,
	attestors []attestor.Attestor,
) (*Set, error) {
	generators := make(map[string]Prover, len(cfg.Relayer.Connections)*2)

	err := forEachClientEnd(cfg, func(connAlias string, self, counterparty config.ClientEnd) error {
		gen, err := attestation.ResolveGenerator(ctx, self, counterparty, clientSet, attestors)
		if err != nil {
			return errors.Wrapf(err, "connection %q", connAlias)
		}

		generators[Key(self.ChainID, self.ClientID)] = gen

		return nil
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
	generators map[string]Prover,
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
	case config.ClientTypeRemoteProver:
		// Nothing to resolve locally: the relayer only knows the endpoint, and
		// the service decides how the client is proven.
		generators[Key(client.ChainID, client.ClientID)] = remote.NewFromURL(
			client.ProverURL, client.ChainID, client.ClientID,
		)

		return nil
	default:
		return errors.Errorf("connection %q: unsupported client type %q for proof generation", connAlias, client.Type)
	}
}
