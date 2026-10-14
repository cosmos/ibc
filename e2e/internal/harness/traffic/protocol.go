// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"maps"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

const (
	ChainA environment.ChainID = "chain-a"
	ChainB environment.ChainID = "chain-b"
)

const ProtocolAuthorityID environment.AuthorityID = "protocol-deployer"

// Deterministic deployer key used by protocol realization; funded by managed Chains.
const protocolAuthorityKeyHex = "0000000000000000000000000000000000000000000000000000000000000005"

// RuntimeWithProtocolDeployer returns runtime with the protocol deployer
// authority, without retaining caller-owned maps.
func RuntimeWithProtocolDeployer(runtime environment.Runtime) environment.Runtime {
	runtime.Endpoints = maps.Clone(runtime.Endpoints)
	runtime.Authorities = maps.Clone(runtime.Authorities)
	if runtime.Authorities == nil {
		runtime.Authorities = map[environment.AuthorityID]environment.EVMAuthority{}
	}
	runtime.Authorities[ProtocolAuthorityID] = environment.EVMAuthority{
		PrivateKeyHex: protocolAuthorityKeyHex,
	}
	return runtime
}
