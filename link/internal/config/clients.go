// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/lightclient"
)

// ValidateClients checks built-in attestation parameters and validates custom
// client ends through the caller-supplied registry.
//
// Every entry point that builds a relayer must call this.
func (c Config) ValidateClients(reg *lightclient.Registry) error {
	if _, shadowsBuiltin := reg.Get(string(ClientTypeAttestation)); shadowsBuiltin {
		return errors.Errorf("client type %q is built in and cannot be overridden", ClientTypeAttestation)
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

func validateClientEnd(reg *lightclient.Registry, end ClientEnd) error {
	if end.Type == ClientTypeAttestation {
		if !end.ClientParams.IsEmpty() {
			return errors.New(".clientParams attestation clients take no params; configure attestors under .attestors")
		}

		return nil
	}

	factory, ok := reg.Get(string(end.Type))
	if !ok {
		registered := append([]string{string(ClientTypeAttestation)}, reg.Types()...)
		return errors.Errorf(
			".type unknown client type %q (registered: [%s])",
			end.Type,
			strings.Join(registered, ", "),
		)
	}

	if err := factory.ValidateParams(end.ClientParams); err != nil {
		return errors.Wrap(err, ".clientParams")
	}

	return nil
}
