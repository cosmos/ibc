package config

import (
	"github.com/pkg/errors"
)

// AttestorConfig represents the entrypoint for running the process as an attestor
type AttestorConfig struct {
	Attestations []AttestationConfig `yaml:"attestations"`
}

// AttestationConfig represents a single attestation configuration in case when the binary
// runs attestors. Signer is a reference to .singers section in the config (future)
// Name should be unique within the config.
type AttestationConfig struct {
	ChainID string `yaml:"chainId"`
	Name    string `yaml:"name"`

	// todo: future work
	RouterAddress  string `yaml:"-"`
	FinalityOffset int64  `yaml:"-"`
	Signer         string `yaml:"-"`
}

// Validate validates the attestor config. Allows empty attestations.
func (c AttestorConfig) Validate() error {
	set := make(map[string]struct{})

	for _, attestation := range c.Attestations {
		if attestation.ChainID == "" {
			return errors.Errorf(".attestations chainId required")
		}
		if attestation.Name == "" {
			return errors.Errorf(".attestations name required")
		}
		if _, ok := set[attestation.Name]; ok {
			return errors.Errorf(".attestations duplicate name: %q", attestation.Name)
		}
		set[attestation.Name] = struct{}{}
	}

	return nil
}
