// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/proofgen"
)

// ValidateClients is the second validation stage: it checks that every
// configured client end names a registered light client type and that its
// params satisfy that type's factory.
//
// It is separate from Validate because it needs a populated registry, and the
// attestation factory can only be built once attestors have been resolved from
// the config Validate already checked. Keeping the structural checks in
// Validate means a malformed database URL or unknown signer alias still fails
// immediately at load, before any chain access.
//
// Every entry point that builds a relayer must call this; link/app does it for
// external callers.
func (c Config) ValidateClients(reg *proofgen.Registry) error {
	if reg == nil {
		return errors.New("client type registry required")
	}

	for _, conn := range c.Relayer.Connections {
		for name, end := range map[string]ClientEnd{"clientA": conn.ClientA, "clientB": conn.ClientB} {
			if err := validateClientEnd(reg, end); err != nil {
				return errors.Wrapf(err, ".relayer.connections[%s].%s", conn.Alias, name)
			}
		}
	}

	return nil
}

func validateClientEnd(reg *proofgen.Registry, end ClientEnd) error {
	factory, ok := reg.Get(string(end.Type))
	if !ok {
		return errors.Errorf(
			".type unknown client type %q (registered: [%s])",
			end.Type,
			strings.Join(reg.Types(), ", "),
		)
	}

	if err := factory.ValidateParams(end.Params); err != nil {
		return errors.Wrap(err, ".params")
	}

	return nil
}
