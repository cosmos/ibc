// SPDX-License-Identifier: Apache-2.0

package attestation

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/cli/internal/chains"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/service/attestor"
)

// ResolveGenerator builds a Generator for self, tracking counterparty, once
// enough attestors satisfy self's on-chain attestation set to meet its
// threshold.
func ResolveGenerator(
	ctx context.Context,
	self, counterparty config.ClientEnd,
	clientSet *chains.ClientSet,
	attestors []attestor.Attestor,
	logger *slog.Logger,
) (*Generator, error) {
	matched, set, err := matchAttestors(ctx, self, counterparty, clientSet, attestors)
	if err != nil {
		return nil, err
	}

	counterpartyChain, ok := clientSet.Get(counterparty.ChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for counterparty chain %q", counterparty.ChainID)
	}

	return newGenerator(self.ChainID, self.ClientID, matched, set, counterpartyChain, logger), nil
}

// matchAttestors resolves self's on-chain attestation set and returns the
// attestors that watch counterparty's chain and whose self-reported address
// appears in it, along with the set, erroring if too few match to meet its
// threshold.
func matchAttestors(
	ctx context.Context,
	self, counterparty config.ClientEnd,
	clientSet *chains.ClientSet,
	attestors []attestor.Attestor,
) ([]attestor.Attestor, *attestationSet, error) {
	selfChain, ok := clientSet.Get(self.ChainID)
	if !ok {
		return nil, nil, errors.Errorf("client %q: no configured chain client for %q", self.ClientID, self.ChainID)
	}

	onChainAddrs, minRequiredSigs, err := selfChain.GetAttestationSet(ctx, self.ClientID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "querying on-chain attestation set for client %q", self.ClientID)
	}

	set := &attestationSet{
		signers:   make(map[common.Address]struct{}, len(onChainAddrs)),
		threshold: int(minRequiredSigs),
	}
	for _, addr := range onChainAddrs {
		set.signers[common.HexToAddress(addr)] = struct{}{}
	}

	var matched []attestor.Attestor

	configured := make([]string, 0, len(attestors))
	seenAddresses := make(map[common.Address]string, len(attestors))

	for _, a := range attestors {
		if a.ChainID() != counterparty.ChainID {
			continue
		}

		address := common.HexToAddress(a.Address())

		if existingName, dup := seenAddresses[address]; dup {
			return nil, nil, errors.Errorf(
				"client %q: attestors %q and %q share address %q watching chain %q",
				self.ClientID, existingName, a.Name(), a.Address(), counterparty.ChainID,
			)
		}
		seenAddresses[address] = a.Name()

		configured = append(configured, fmt.Sprintf("%s=%s", a.Name(), a.Address()))

		if _, inOnChainSet := set.signers[address]; !inOnChainSet {
			continue
		}

		matched = append(matched, a)
	}

	if len(matched) < set.threshold {
		return nil, nil, errors.Errorf(
			"client %q: only %d reachable/matching attestors for chain %q, on-chain quorum requires %d "+
				"(on-chain addresses: [%s], configured attestors: [%s])",
			self.ClientID, len(matched), counterparty.ChainID, set.threshold,
			strings.Join(onChainAddrs, ", "), strings.Join(configured, ", "),
		)
	}

	return matched, set, nil
}
