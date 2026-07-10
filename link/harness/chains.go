package harness

import (
	"errors"
	"fmt"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/provision"
	"github.com/cosmos/ibc/link/harness/topology"
)

// Chains is the registry of one run's provisioned chains, keyed by logical chain id. Each entry carries
// the chain handle, its resolved timing profile, and its lifecycle hooks — the per-chain runtime state,
// as opposed to the run-wide state (driver, bundle, workspace) the Harness holds.
type Chains struct {
	list []provision.Provisioned // start order; teardown stops in reverse
	byID map[string]provision.Provisioned
}

// newChains indexes the provisioned list by chain id. Uniqueness is guaranteed upstream
// (provision.Start rejects duplicate ids before launching anything).
func newChains(list []provision.Provisioned) *Chains {
	byID := make(map[string]provision.Provisioned, len(list))
	for _, p := range list {
		byID[p.Chain.ID()] = p
	}
	return &Chains{list: list, byID: byID}
}

// Get returns the chain with id, or a clear error when id names no chain in the run — never a nil
// interface a caller could deref.
func (c *Chains) Get(id string) (chain.Chain, error) {
	p, ok := c.byID[id]
	if !ok {
		return nil, fmt.Errorf("harness: no chain %q", id)
	}
	return p.Chain, nil
}

// EVM returns chain id's concrete EVM client (the evm.ClientProvider capability), or an error if the
// chain is missing or not EVM-family.
func (c *Chains) EVM(id string) (*evm.EVMClient, error) {
	ch, err := c.Get(id)
	if err != nil {
		return nil, err
	}
	ec, ok := evm.FromChain(ch)
	if !ok {
		return nil, fmt.Errorf("harness: chain %q is not an EVM chain", id)
	}
	return ec, nil
}

// Profile returns the resolved timing profile for id — the budget source for every wait observing that
// chain. An unknown id is a programmer error (every path here resolves the id through Get, the
// deployment, or the topology first), so it panics rather than substituting a default: a silently
// wrong profile is worse than a loud failure, since a too-short settle window can green a stability
// check the observed chain's real cadence would fail.
func (c *Chains) Profile(id string) topology.TimingProfile {
	p, ok := c.byID[id]
	if !ok {
		panic(
			fmt.Sprintf(
				"harness: Profile(%q): no such chain — every wait budget must derive from a provisioned chain's resolved profile",
				id,
			),
		)
	}
	return p.Profile
}

// stopAll stops every chain in reverse start order, joining any stop errors.
func (c *Chains) stopAll() error {
	var errs []error
	for i := len(c.list) - 1; i >= 0; i-- {
		p := c.list[i]
		if err := p.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop chain %s: %w", p.Chain.ID(), err))
		}
	}
	return errors.Join(errs...)
}
