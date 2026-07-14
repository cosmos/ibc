package wire

// TestAppDeployment records the test applications deployed on each configured
// chain. It is not an IBC protocol deployment or connection receipt.
type TestAppDeployment struct {
	Chains map[string]ChainTestAppDeployment `json:"chains"`
}

type ChainTestAppDeployment struct {
	MockIFT string `json:"mockIFT"`
	MockGMP string `json:"mockGMP"`
	Counter string `json:"counter"`
	TxHash  string `json:"txHash"`
}

func (d *TestAppDeployment) Chain(id string) (ChainTestAppDeployment, bool) {
	apps, ok := d.Chains[id]
	return apps, ok
}
