// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/harness/environment"
)

func TestRuntimeWithProtocolDeployer(t *testing.T) {
	input := environment.Runtime{
		Endpoints: map[environment.EndpointBindingID]environment.EndpointBinding{
			"rpc": {RPCURL: "http://example.test"},
		},
		Authorities: map[environment.AuthorityID]environment.EVMAuthority{
			ProtocolAuthorityID: {PrivateKeyHex: "old"},
			"other":             {PrivateKeyHex: "other-key"},
		},
	}
	got := RuntimeWithProtocolDeployer(input)
	got.Endpoints["new"] = environment.EndpointBinding{RPCURL: "http://new.test"}
	got.Authorities["new"] = environment.EVMAuthority{PrivateKeyHex: "new-key"}

	require.Equal(t, protocolAuthorityKeyHex, got.Authorities[ProtocolAuthorityID].PrivateKeyHex)
	require.Equal(t, "old", input.Authorities[ProtocolAuthorityID].PrivateKeyHex)
	require.Empty(t, input.Authorities["new"].PrivateKeyHex)
	require.Empty(t, input.Endpoints["new"].RPCURL)
	require.Equal(t, input.Endpoints["rpc"], got.Endpoints["rpc"])
}
