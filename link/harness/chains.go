package harness

import (
	"errors"
	"fmt"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/internal/provision"
	"github.com/cosmos/ibc/link/harness/topology"
)

type Chains struct {
	list []provision.Provisioned
	byID map[string]provision.Provisioned
}

func newChains(list []provision.Provisioned) *Chains {
	byID := make(map[string]provision.Provisioned, len(list))
	for _, p := range list {
		byID[p.Chain.ID()] = p
	}
	return &Chains{list: list, byID: byID}
}

func (c *Chains) Get(id string) (chain.Chain, error) {
	p, ok := c.byID[id]
	if !ok {
		return nil, fmt.Errorf("harness: no chain %q", id)
	}
	return p.Chain, nil
}

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

// Unknown ids panic: a silently wrong wait budget is worse than a loud failure.
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
