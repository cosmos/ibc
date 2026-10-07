// SPDX-License-Identifier: Apache-2.0

package attestor

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/service/signer"
)

// ResolveOptions controls how unresolvable remote attestors are handled.
type ResolveOptions struct {
	// RequireReachable fails on the first remote attestor that can't be
	// resolved, as needed by live validation. Without it such an attestor is
	// skipped with a warning.
	RequireReachable bool
}

// ResolveFromConfig resolves every entry in the unified attestors[] config
// list into a live Attestor, split by whether it runs in this process
// (local) or is queried over gRPC (remote).
func ResolveFromConfig(
	ctx context.Context,
	entries config.Attestors,
	clients *chains.ClientSet,
	signers *signer.Set,
	opts ResolveOptions,
) (local, remote []Attestor, err error) {
	for _, entry := range entries {
		switch entry.Type {
		case config.AttestorTypeLocal:
			a, errLocal := resolveLocal(entry, clients, signers)
			if errLocal != nil {
				return nil, nil, fmt.Errorf("attestor %s: %w", entry.Name, errLocal)
			}

			local = append(local, a)
		case config.AttestorTypeRemote:
			a, errRemote := resolveRemote(ctx, entry)
			if errRemote != nil {
				if opts.RequireReachable {
					return nil, nil, fmt.Errorf("attestor %s: %w", entry.Name, errRemote)
				}

				slog.Warn(
					"Skipping unresolvable configured attestor",
					"name", entry.Name, "type", entry.Type, "err", errRemote,
				)
				continue
			}

			remote = append(remote, a)
		default:
			slog.Warn("Skipping unsupported configured attestor type", "name", entry.Name, "type", entry.Type)
		}
	}

	return local, remote, nil
}

func resolveLocal(entry config.AttestorConfig, clients *chains.ClientSet, signers *signer.Set) (Attestor, error) {
	client, ok := clients.Get(entry.ChainID)
	if !ok {
		return nil, fmt.Errorf("client not found for chain %s", entry.ChainID)
	}

	s, ok := signers.Get(entry.Signer)
	if !ok {
		return nil, fmt.Errorf("unknown signer %s", entry.Signer)
	}

	return NewLocal(entry, client, s)
}

func resolveRemote(ctx context.Context, entry config.AttestorConfig) (Attestor, error) {
	if entry.GRPC == "" {
		return nil, errors.New("no grpc address configured")
	}

	// grpc is a bare host:port, so the tls block's presence picks the scheme.
	scheme := "http://"
	if entry.TLS != nil {
		scheme = "https://"
	}

	endpoint, err := config.ResolveEndpoint(
		scheme+entry.GRPC, entry.TLS, nil, "attestor", entry.Name, "grpc", entry.GRPC,
	)
	if err != nil {
		return nil, err
	}

	return NewRemoteFromEndpoint(ctx, entry.Name, endpoint)
}
