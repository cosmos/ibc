package harness

import (
	"errors"

	"github.com/cosmos/ibc/link/harness/topology"
)

// StartConfig configures the harness startup.
type StartConfig struct {
	// Topology is the harness environment to start (required).
	Topology topology.Topology

	// KeepOnClose, if true, leaves the whole world running after Shutdown so a failure can be inspected:
	// the chains, the workdir (sqlite, compiled config, logs), and the relayer daemon (its pid file in the
	// workdir names the process to sweep). Diagnostics are still captured and artifacts still written.
	KeepOnClose bool

	// ArtifactDir is the directory where failure diagnostics are written.
	// If empty, no artifact file is written.
	ArtifactDir string
}

// Validate reports whether the required fields are present.
func (c StartConfig) Validate() error {
	if c.Topology.Name == "" {
		return errors.New("harness: StartConfig.Topology is required")
	}
	return nil
}
