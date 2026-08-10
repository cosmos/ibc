package attestation

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/service/attestor"
)

// ResolveGenerator builds a Generator for self, tracking counterparty, once
// enough attestors satisfy self's on-chain attestation set to meet its
// threshold.
func ResolveGenerator(
	ctx context.Context,
	self, counterparty config.ClientEnd,
	clientSet *chains.ClientSet,
	attestors []attestor.Attestor,
) (*Generator, error) {
	matched, minRequiredSigs, err := MatchAttestors(ctx, self, counterparty, clientSet, attestors)
	if err != nil {
		return nil, err
	}

	counterpartyChain, ok := clientSet.Get(counterparty.ChainID)
	if !ok {
		return nil, errors.Errorf("no configured chain client for counterparty chain %q", counterparty.ChainID)
	}

	return New(matched, int(minRequiredSigs), counterpartyChain), nil
}

// MatchAttestors resolves self's on-chain attestation set and returns the
// attestors that watch counterparty's chain and whose self-reported address
// appears in it, erroring if too few match to meet the on-chain threshold.
func MatchAttestors(
	ctx context.Context,
	self, counterparty config.ClientEnd,
	clientSet *chains.ClientSet,
	attestors []attestor.Attestor,
) ([]attestor.Attestor, uint8, error) {
	selfChain, ok := clientSet.Get(self.ChainID)
	if !ok {
		return nil, 0, errors.Errorf("client %q: no configured chain client for %q", self.ClientID, self.ChainID)
	}

	onChainAddrs, minRequiredSigs, err := selfChain.GetAttestationSet(ctx, self.ClientID)
	if err != nil {
		return nil, 0, errors.Wrapf(err, "querying on-chain attestation set for client %q", self.ClientID)
	}

	onChainSet := make(map[string]struct{}, len(onChainAddrs))
	for _, addr := range onChainAddrs {
		onChainSet[strings.ToLower(addr)] = struct{}{}
	}

	var matched []attestor.Attestor

	configured := make([]string, 0, len(attestors))

	for _, a := range attestors {
		if a.ChainID() != counterparty.ChainID {
			continue
		}

		configured = append(configured, fmt.Sprintf("%s=%s", a.Name(), a.Address()))

		if _, inOnChainSet := onChainSet[strings.ToLower(a.Address())]; !inOnChainSet {
			continue
		}

		matched = append(matched, a)
	}

	if len(matched) < int(minRequiredSigs) {
		return nil, 0, errors.Errorf(
			"client %q: only %d reachable/matching attestors for chain %q, on-chain quorum requires %d "+
				"(on-chain addresses: [%s], configured attestors: [%s])",
			self.ClientID, len(matched), counterparty.ChainID, minRequiredSigs,
			strings.Join(onChainAddrs, ", "), strings.Join(configured, ", "),
		)
	}

	return matched, minRequiredSigs, nil
}
