package wire

import "fmt"

// Deployment is the machine-readable metadata the stub's `deploy` command emits on stdout: the
// per-chain fixture addresses, mock client ids, and tx hashes the deploy produced. Routes live in the
// topology/config; deployment reports only artifacts that deploy actually created.
type Deployment struct {
	Chains   map[string]ChainDeployment `json:"chains"` // keyed by Chain.ID
	TxHashes []string                   `json:"txHashes"`
}

// ChainDeployment is the set of fixtures deployed on one chain, keyed by name. ClientID is the light-client
// id the deploy assigns to the chain pairing.
//
// Fixtures maps a well-known fixture name to that fixture's on-chain address in the chain family's own
// native string form (EVM hex today). The map — rather than named EVM-shaped fields — is the seam a
// non-EVM family reuses: it records its own fixtures under the same names without the DTO carrying an
// address-shape assumption. The names themselves are not part of this generic contract: the harness-owned
// fixturekeys package defines the current (mock) vocabulary that the deploy writes and the readers read.
type ChainDeployment struct {
	Fixtures map[string]string `json:"fixtures"` // fixture name -> native address string
	ClientID string            `json:"clientId"` // mock client id for the chain pairing
}

func (d *Deployment) Chain(id string) (ChainDeployment, bool) {
	c, ok := d.Chains[id]
	return c, ok
}

// Fixture resolves a named fixture's address on this chain deployment. A missing (or empty) entry is a
// clear error naming the fixture, not an empty string that silently coerces to a zero address downstream;
// callers with a chain id wrap the error to name the chain too.
func (c ChainDeployment) Fixture(name string) (string, error) {
	addr, ok := c.Fixtures[name]
	if !ok || addr == "" {
		return "", fmt.Errorf("wire: chain deployment has no %q fixture", name)
	}
	return addr, nil
}
