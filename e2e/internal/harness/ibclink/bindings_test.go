package ibclink

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChainRPCBindingKeepsEndpointOutOfConfiguration(t *testing.T) {
	const (
		chainID  = "chain/a"
		endpoint = "https://user:secret@rpc.example.invalid"
	)
	driver, err := NewDriver("/tmp/ibc-link-config.yaml")
	require.NoError(t, err)
	require.NoError(t, driver.BindChainRPCs(map[string]func() (string, error){
		chainID: func() (string, error) { return endpoint, nil },
	}, passthroughProcessLease))

	rpc, err := driver.ChainRPC(chainID)
	require.NoError(t, err)
	require.NotContains(t, rpc.URL, endpoint)
	require.Equal(t, "${"+chainRPCEnvName(chainID)+"}", rpc.URL)

	var env []string
	err = driver.withProcessEnv(func(got processEnvironment) error {
		env = got.variables
		return nil
	})
	require.NoError(t, err)
	require.Contains(t, env, chainRPCEnvName(chainID)+"="+endpoint)
}

func TestChainRPCBindingPropagatesResolverFailureWithoutEndpointData(t *testing.T) {
	closed := errors.New("environment closed")
	driver, err := NewDriver("/tmp/ibc-link-config.yaml")
	require.NoError(t, err)
	require.NoError(t, driver.BindChainRPCs(map[string]func() (string, error){
		"chain-a": func() (string, error) { return "", closed },
	}, passthroughProcessLease))

	rpc, err := driver.ChainRPC("chain-a")
	require.NoError(t, err)
	require.NotEmpty(t, rpc.URL)
	err = driver.withProcessEnv(func(processEnvironment) error { return nil })
	require.ErrorIs(t, err, closed)
}

func TestChainRPCBindingsInstallOnce(t *testing.T) {
	driver, err := NewDriver("/tmp/ibc-link-config.yaml")
	require.NoError(t, err)
	resolver := func() (string, error) { return "http://rpc.invalid", nil }
	require.NoError(t, driver.BindChainRPCs(
		map[string]func() (string, error){"chain-a": resolver},
		passthroughProcessLease,
	))
	require.ErrorContains(
		t,
		driver.BindChainRPCs(
			map[string]func() (string, error){"chain-b": resolver},
			passthroughProcessLease,
		),
		"already installed",
	)

	rpc, err := driver.ChainRPC("chain-a")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(rpc.URL, "${"+chainRPCEnvPrefix))
	_, err = driver.ChainRPC("chain-b")
	require.ErrorContains(t, err, `no RPC binding for Chain "chain-b"`)
}

func passthroughProcessLease() (func(), error) { return func() {}, nil }
