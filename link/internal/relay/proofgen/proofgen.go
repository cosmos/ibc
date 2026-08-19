// SPDX-License-Identifier: Apache-2.0

// Package proofgen resolves configured light-client provers.
package proofgen

import (
	"context"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/proofgen/attestation"
	attestorservice "github.com/cosmos/ibc/link/internal/service/attestor"
	"github.com/cosmos/ibc/link/lightclient"
)

// Key identifies a client on a chain.
func Key(chainID, clientID string) string {
	return chainID + "/" + clientID
}

// Set resolves provers by chain and client ID.
type Set struct {
	generators map[string]lightclient.Prover
}

func NewSet(generators map[string]lightclient.Prover) *Set {
	if generators == nil {
		generators = make(map[string]lightclient.Prover)
	}

	return &Set{generators: generators}
}

func (s *Set) Get(chainID, clientID string) (lightclient.Prover, bool) {
	generator, ok := s.generators[Key(chainID, clientID)]
	return generator, ok
}

// NewSetFromConfig resolves provers for all configured client ends.
func NewSetFromConfig(
	ctx context.Context,
	cfg config.Config,
	clientSet *chains.ClientSet,
	attestors []attestorservice.Attestor,
	reg *lightclient.Registry,
) (*Set, error) {
	generators := make(map[string]lightclient.Prover, len(cfg.Relayer.Connections)*2)
	chainInfos := make(map[string]lightclient.ChainInfo, len(cfg.Chains))
	for _, chain := range cfg.Chains {
		chainInfos[chain.ChainID] = toChainInfo(chain)
	}

	err := forEachClientEnd(cfg, func(connAlias string, self, counterparty config.ClientEnd) error {
		return addGenerator(ctx, generators, connAlias, self, counterparty, chainInfos, clientSet, attestors, reg)
	})
	if err != nil {
		return nil, err
	}

	return NewSet(generators), nil
}

// forEachClientEnd calls fn for both ends of every connection.
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
	generators map[string]lightclient.Prover,
	connAlias string,
	client, clientCounterparty config.ClientEnd,
	chainInfo map[string]lightclient.ChainInfo,
	clientSet *chains.ClientSet,
	attestors []attestorservice.Attestor,
	reg *lightclient.Registry,
) error {
	if client.Type == config.ClientTypeAttestation {
		gen, err := attestation.ResolveGenerator(
			ctx, client, clientCounterparty, clientSet, attestors,
		)
		if err != nil {
			return err
		}

		generators[Key(client.ChainID, client.ClientID)] = gen

		return nil
	}

	factory, ok := reg.Get(string(client.Type))
	if !ok {
		registered := append([]string{string(config.ClientTypeAttestation)}, reg.Types()...)
		return errors.Errorf(
			"connection %q: no proof generator registered for client type %q (registered: %v)",
			connAlias, client.Type, registered,
		)
	}
	hostChain, ok := chainInfo[client.ChainID]
	if !ok {
		return errors.Errorf("connection %q: no chain config for host chain %q", connAlias, client.ChainID)
	}
	counterpartyChain, ok := chainInfo[clientCounterparty.ChainID]
	if !ok {
		return errors.Errorf(
			"connection %q: no chain config for counterparty chain %q", connAlias, clientCounterparty.ChainID,
		)
	}

	gen, err := factory.New(
		ctx,
		lightclient.ProverFactoryOptions{
			Client:            toClientInfo(client),
			HostChain:         hostChain,
			CounterpartyChain: counterpartyChain,
		},
	)
	if err != nil {
		return errors.Wrapf(err, "connection %q", connAlias)
	}

	generators[Key(client.ChainID, client.ClientID)] = gen

	return nil
}

func toChainInfo(chain config.ChainConfig) lightclient.ChainInfo {
	info := lightclient.ChainInfo{ChainID: chain.ChainID}
	if chain.EVM != nil {
		info.EVM = &lightclient.EVMChainInfo{
			RPC:         chain.EVM.RPC,
			ICS26Router: chain.EVM.ICS26Router,
		}
	}

	return info
}

func toClientInfo(end config.ClientEnd) lightclient.ClientInfo {
	return lightclient.ClientInfo{
		ClientID:     end.ClientID,
		Type:         string(end.Type),
		ClientParams: end.ClientParams,
	}
}
