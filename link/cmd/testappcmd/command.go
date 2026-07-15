// Package testappcmd owns the test-app command's CLI and transport contract.
package testappcmd

import "github.com/spf13/cobra"

// Handler implements a test-app subcommand.
type Handler func(*cobra.Command, []string) error

// NewCommand constructs the test-app command with its behavior injected by the executable.
func NewCommand(handler Handler) *cobra.Command {
	cmd := &cobra.Command{Use: "test-apps", Short: "Synthetic test application commands"}
	cmd.AddCommand(&cobra.Command{
		Use:          "deploy",
		Short:        "Deploy the synthetic test applications",
		SilenceUsage: true,
		RunE:         handler,
	})
	return cmd
}

// Deployment records the test applications deployed on each configured chain.
type Deployment struct {
	Chains map[string]ChainDeployment `json:"chains"`
}

// ChainDeployment records one chain's synthetic applications.
type ChainDeployment struct {
	MockIFT string `json:"mockIFT"`
	MockGMP string `json:"mockGMP"`
	Counter string `json:"counter"`
	TxHash  string `json:"txHash"`
}

// Chain returns a chain deployment by ID.
func (d *Deployment) Chain(id string) (ChainDeployment, bool) {
	apps, ok := d.Chains[id]
	return apps, ok
}
