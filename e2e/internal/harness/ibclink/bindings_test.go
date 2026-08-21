// SPDX-License-Identifier: Apache-2.0

package ibclink

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChainEndpointBindingKeepsEndpointsOutOfConfiguration(t *testing.T) {
	const (
		chainID  = "chain/a"
		endpoint = "https://user:secret@rpc.example.invalid"
		wsPoint  = "wss://user:secret@rpc.example.invalid"
	)
	driver, err := NewDriver("/tmp/ibc-link-config.yaml")
	require.NoError(t, err)
	require.NoError(t, driver.BindChainEndpoints(map[string]func() (ChainEndpoints, error){
		chainID: func() (ChainEndpoints, error) { return ChainEndpoints{RPC: endpoint, WS: wsPoint}, nil },
	}, passthroughProcessLease))

	rpc, err := driver.ChainRPC(chainID)
	require.NoError(t, err)
	require.NotContains(t, rpc, endpoint)
	require.Equal(t, "${"+chainRPCEnvName(chainID)+"}", rpc)

	ws, err := driver.ChainWS(chainID)
	require.NoError(t, err)
	require.NotContains(t, ws, wsPoint)
	require.Equal(t, "${"+chainWSEnvName(chainID)+"}", ws)

	env, release, err := driver.acquireProcessEnv()
	require.NoError(t, err)
	defer release()
	require.Contains(t, env, chainRPCEnvName(chainID)+"="+endpoint)
	require.Contains(t, env, chainWSEnvName(chainID)+"="+wsPoint)
}

func TestChainEndpointBindingResolvesEmptyWebsocket(t *testing.T) {
	driver, err := NewDriver("/tmp/ibc-link-config.yaml")
	require.NoError(t, err)
	require.NoError(t, driver.BindChainEndpoints(map[string]func() (ChainEndpoints, error){
		"chain-a": func() (ChainEndpoints, error) { return ChainEndpoints{RPC: "http://rpc.invalid"}, nil },
	}, passthroughProcessLease))

	env, release, err := driver.acquireProcessEnv()
	require.NoError(t, err)
	defer release()
	require.Contains(t, env, chainWSEnvName("chain-a")+"=")
}

func TestChainEndpointBindingPropagatesResolverFailureWithoutEndpointData(t *testing.T) {
	closed := errors.New("environment closed")
	driver, err := NewDriver("/tmp/ibc-link-config.yaml")
	require.NoError(t, err)
	require.NoError(t, driver.BindChainEndpoints(map[string]func() (ChainEndpoints, error){
		"chain-a": func() (ChainEndpoints, error) { return ChainEndpoints{}, closed },
	}, passthroughProcessLease))

	rpc, err := driver.ChainRPC("chain-a")
	require.NoError(t, err)
	require.NotEmpty(t, rpc)
	_, _, err = driver.acquireProcessEnv()
	require.ErrorIs(t, err, closed)
}

func TestChainEndpointBindingsInstallOnce(t *testing.T) {
	driver, err := NewDriver("/tmp/ibc-link-config.yaml")
	require.NoError(t, err)
	resolver := func() (ChainEndpoints, error) { return ChainEndpoints{RPC: "http://rpc.invalid"}, nil }
	require.NoError(t, driver.BindChainEndpoints(
		map[string]func() (ChainEndpoints, error){"chain-a": resolver},
		passthroughProcessLease,
	))
	require.ErrorContains(
		t,
		driver.BindChainEndpoints(
			map[string]func() (ChainEndpoints, error){"chain-b": resolver},
			passthroughProcessLease,
		),
		"already installed",
	)

	rpc, err := driver.ChainRPC("chain-a")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(rpc, "${"+chainRPCEnvPrefix))
	_, err = driver.ChainRPC("chain-b")
	require.ErrorContains(t, err, `no RPC binding for Chain "chain-b"`)
	_, err = driver.ChainWS("chain-b")
	require.ErrorContains(t, err, `no RPC binding for Chain "chain-b"`)
}

func passthroughProcessLease() (func(), error) { return func() {}, nil }
