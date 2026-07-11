package wire

import "fmt"

type Deployment struct {
	Chains   map[string]ChainDeployment `json:"chains"`
	TxHashes []string                   `json:"txHashes"`
}

type ChainDeployment struct {
	Fixtures map[string]string `json:"fixtures"`
	ClientID string            `json:"clientId"`
}

func (d *Deployment) Chain(id string) (ChainDeployment, bool) {
	c, ok := d.Chains[id]
	return c, ok
}

func (c ChainDeployment) Fixture(name string) (string, error) {
	addr, ok := c.Fixtures[name]
	if !ok || addr == "" {
		return "", fmt.Errorf("wire: chain deployment has no %q fixture", name)
	}
	return addr, nil
}
