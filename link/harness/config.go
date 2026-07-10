package harness

import (
	"errors"

	"github.com/cosmos/ibc/link/harness/topology"
)

type StartConfig struct {
	Topology    topology.Topology
	KeepOnClose bool
	ArtifactDir string
}

func (c StartConfig) Validate() error {
	if c.Topology.Name == "" {
		return errors.New("harness: StartConfig.Topology is required")
	}
	return nil
}
