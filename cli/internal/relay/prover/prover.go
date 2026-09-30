// SPDX-License-Identifier: Apache-2.0

// Package prover generates packet membership/non-membership proofs and
// light-client state proofs. There is one implementation per light-client
// type.
package prover

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/relay/prover/attestation"
	"github.com/cosmos/ibc/cli/internal/relay/prover/remote"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
	v2 "github.com/cosmos/ibc/cli/internal/types/v2"
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

// SetOptions controls remote prover checks during construction.
type SetOptions struct {
	// RequireReachable makes probe failures fatal, as needed by live validation.
	RequireReachable bool
}

// NewSetFromConfig resolves a Prover for every client end of every
// configured connection, matching against attestors (this process's own
// local attestors plus every resolved remote one).
//
// Attestation client ends resolve in order, failing fast on the first bad
// one. Remote client ends are only built here, in the same pass; probing
// them for reachability happens afterward, concurrently across all of
// them (see probeRemoteProvers), since a single probe can take up to a
// minute and resolving them one at a time would make that minute multiply
// by the number of remote client ends.
func NewSetFromConfig(
	ctx context.Context,
	cfg config.Config,
	clientSet *chains.ClientSet,
	attestors []attestor.Attestor,
	logger *slog.Logger,
	opts SetOptions,
) (*Set, error) {
	generators := make(map[string]Prover, len(cfg.Relayer.Connections)*2)
	var remoteProvers []remoteProver

	err := forEachClientEnd(cfg, func(connAlias string, self, counterparty config.ClientEnd) error {
		switch self.Type {
		case config.ClientTypeAttestation:
			return addAttestationGenerator(ctx, generators, connAlias, self, counterparty, clientSet, attestors, logger)
		case config.ClientTypeRemote:
			p, err := buildRemoteProver(connAlias, self, logger)
			if err != nil {
				return err
			}

			remoteProvers = append(remoteProvers, p)

			return nil
		default:
			return errors.Errorf("connection %q: unsupported client type %q for proof generation", connAlias, self.Type)
		}
	})
	if err != nil {
		return nil, err
	}

	if err := probeRemoteProvers(ctx, remoteProvers, generators, opts.RequireReachable, logger); err != nil {
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

func addAttestationGenerator(
	ctx context.Context,
	generators map[string]Prover,
	connAlias string,
	client, clientCounterparty config.ClientEnd,
	clientSet *chains.ClientSet,
	attestors []attestor.Attestor,
	logger *slog.Logger,
) error {
	logger = logger.With("module", "prover", "chainID", client.ChainID, "clientID", client.ClientID)

	meteredAttestors := make([]attestor.Attestor, len(attestors))
	for i, a := range attestors {
		meteredAttestors[i] = attestor.MetricsWrapper(a)
	}

	gen, err := attestation.ResolveGenerator(ctx, client, clientCounterparty, clientSet, meteredAttestors, logger)
	if err != nil {
		return errors.Wrapf(err, "connection %q", connAlias)
	}

	generators[Key(client.ChainID, client.ClientID)] = metricsWrapper(gen, client.ChainID, client.ClientID, client.Type)

	return nil
}

// remoteProver is a remote client end resolved into a live *remote.Prover,
// not yet probed for reachability.
type remoteProver struct {
	connAlias string
	client    config.ClientEnd
	prover    *remote.Prover
}

func buildRemoteProver(connAlias string, client config.ClientEnd, logger *slog.Logger) (remoteProver, error) {
	logger = logger.With("module", "prover", "chainID", client.ChainID, "clientID", client.ClientID)

	params, err := client.ClientParams()
	if err != nil {
		return remoteProver{}, errors.Wrapf(err, "connection %q", connAlias)
	}

	remoteParams, ok := params.(*config.RemoteParams)
	if !ok {
		return remoteProver{}, errors.Errorf("connection %q: %T is not remote prover params", connAlias, params)
	}

	tlsConfig, err := remoteParams.TLS.TLSConfig()
	if err != nil {
		return remoteProver{}, errors.Wrapf(err, "connection %q: tls", connAlias)
	}

	if remoteParams.TLS != nil && remoteParams.TLS.InsecureSkipVerify {
		// Not logging the URL: RemoteParams.Validate doesn't reject userinfo,
		// so it can carry a credential. chainID/clientID (via logger) and
		// connAlias already identify the endpoint without that risk.
		logger.Warn("TLS server certificate verification is disabled", "connection", connAlias)
	}

	prover := remote.NewFromURL(remoteParams.URL, client.ChainID, client.ClientID, tlsConfig, logger)

	return remoteProver{connAlias: connAlias, client: client, prover: prover}, nil
}

// probeRemoteProvers probes every remote prover's reachability concurrently
// and, for the ones that are reachable (or whose unreachability isn't
// fatal), registers them in generators. A single probe can take up to a
// minute (see probeRemote), so probing them one at a time would make that
// minute multiply by len(provers).
func probeRemoteProvers(
	ctx context.Context,
	provers []remoteProver,
	generators map[string]Prover,
	requireReachable bool,
	logger *slog.Logger,
) error {
	var (
		mu sync.Mutex
		g  errgroup.Group
	)

	for _, p := range provers {
		g.Go(func() error {
			if err := probeRemote(ctx, p.prover, requireReachable); err != nil {
				if requireReachable {
					return errors.Wrapf(
						err,
						"connection %q: remote prover %s/%s",
						p.connAlias,
						p.client.ChainID,
						p.client.ClientID,
					)
				}

				logger.Warn("Remote prover unreachable at startup, continuing", "connection", p.connAlias, "err", err)
			}

			meteredProver := metricsWrapper(p.prover, p.client.ChainID, p.client.ClientID, p.client.Type)

			mu.Lock()
			generators[Key(p.client.ChainID, p.client.ClientID)] = meteredProver
			mu.Unlock()

			return nil
		})
	}

	return g.Wait()
}

func probeRemote(ctx context.Context, p *remote.Prover, requireReachable bool) error {
	if !requireReachable {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	return p.Probe(ctx)
}
