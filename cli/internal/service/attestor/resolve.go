// SPDX-License-Identifier: Apache-2.0

package attestor

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/network"
	"github.com/cosmos/ibc/cli/internal/service/signer"
)

// ResolveOptions controls how unreachable remote attestors are handled.
type ResolveOptions struct {
	// RequireReachable fails on the first remote attestor that can't be
	// reached, as needed by live validation. Without it such an attestor is
	// skipped with a warning.
	RequireReachable bool
}

// ResolveFromConfig resolves every entry in the unified attestors[] config
// list into a live Attestor, split by whether it runs in this process
// (local) or is queried over gRPC (remote). A bad remote endpoint config is
// always an error; an unreachable remote attestor is handled per opts.
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
			endpoint, errEndpoint := remoteEndpoint(entry)
			if errEndpoint != nil {
				return nil, nil, fmt.Errorf("attestor %s: %w", entry.Name, errEndpoint)
			}

			a, errRemote := NewRemoteFromEndpoint(ctx, entry.Name, endpoint)
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

// remoteEndpoint resolves entry's address and TLS settings. The name, not the
// address, identifies it in logs: with validation off the address may carry
// userinfo.
func remoteEndpoint(entry config.AttestorConfig) (network.Endpoint, error) {
	if entry.GRPC == "" {
		return network.Endpoint{}, errors.New("no grpc address configured")
	}

	return config.ResolveEndpoint(entry.EndpointURL(), entry.TLS, nil, "attestor", entry.Name)
}
