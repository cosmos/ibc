// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestWriteICS26Routers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ibc.yml")
	require.NoError(t, os.WriteFile(path, []byte(`# kept
chains:
  - chainId: "1"
    evm:
      rpc: http://a
      ics26Router: ""
  - chainId: "2"
    evm:
      rpc: http://b
signers:
  - alias: deployer
    type: local
    file: deployer.json
  - alias: attestor
    type: local
    file: attestor.json
`), 0o600))

	routerA := "0x00000000000000000000000000000000000000aa"
	routerB := "0x00000000000000000000000000000000000000bb"
	require.NoError(t, WriteICS26Routers(path, map[string]string{"1": routerA, "2": routerB}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "# kept")
	var file fileConfig
	require.NoError(t, yaml.Unmarshal(data, &file))
	require.Equal(t, routerA, file.Chains[0].EVM.ICS26Router)
	require.Equal(t, routerB, file.Chains[1].EVM.ICS26Router)
	require.Equal(t, "http://a", file.Chains[0].EVM.RPC)
	require.Len(t, file.Signers, 2)
}
