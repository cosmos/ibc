// SPDX-License-Identifier: Apache-2.0

// Package proofgen generates packet membership/non-membership proofs and
// light-client state proofs. There is one implementation per light-client
// type.
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

// ProofGenerator is the public light-client proof generation contract.
// Keep the alias here so existing internal relay code can continue to refer to
// the package that owns generator resolution and lookup.
type ProofGenerator = lightclient.ProofGenerator

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
// configured connection. Attestation uses its built-in resolver; other client
// types dispatch through the caller-supplied registry.
func NewSetFromConfig(
	ctx context.Context,
	cfg config.Config,
	clientSet *chains.ClientSet,
	attestors []attestorservice.Attestor,
	reg *lightclient.Registry,
) (*Set, error) {
	generators := make(map[string]ProofGenerator, len(cfg.Relayer.Connections)*2)
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
		lightclient.FactoryOptions{
			Client:            toClientInfo(client, clientCounterparty.ChainID),
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

func toClientInfo(end config.ClientEnd, counterpartyChainID string) lightclient.ClientInfo {
	return lightclient.ClientInfo{
		ChainID:             end.ChainID,
		CounterpartyChainID: counterpartyChainID,
		ClientID:            end.ClientID,
		Type:                string(end.Type),
		ClientParams:        end.ClientParams,
	}
}
