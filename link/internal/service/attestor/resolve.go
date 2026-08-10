package attestor

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/signer"
)

// ResolveFromConfig resolves every entry in the unified attestors[] config
// list into a live Attestor, split by whether it runs in this process
// (local) or is queried over gRPC (remote).
//
// Local entries are resolved strictly: this process is meant to run them,
// so a bad client/signer reference is a fatal misconfiguration. Remote
// entries are resolved best-effort: an unreachable remote is excluded and
// logged rather than failing the whole resolution, since a relayer's
// candidate pool should degrade gracefully instead of refusing to start
// over one unreachable peer.
func ResolveFromConfig(
	ctx context.Context,
	entries config.Attestors,
	clients *chains.ClientSet,
	signers *signer.Set,
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

	// TODO: Support TLS
	return NewRemoteFromURL(ctx, "http://"+entry.GRPC, entry.Name)
}
