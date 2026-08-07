package environment_test

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	"github.com/cosmos/ibc/link/keyfile"
)

const relayerStopTimeout = 15 * time.Second

// TestStartFailsWhenConfiguredAttestorsDoNotSatisfyOnChainQuorum proves the
// relayer's startup-time attestor quorum resolution (internal/relay/proofgen
// in the link module) actually blocks a misconfigured relayer rather than
// starting up degraded: clientA's on-chain attestation set requires 2
// signatures, backed by two real, independently-keyed attestor processes. A
// relayer config that only knows about one of them must fail to start; the
// same config with both configured must start cleanly.
func TestStartFailsWhenConfiguredAttestorsDoNotSatisfyOnChainQuorum(t *testing.T) {
	requireDocker(t)
	requireIBCLinkBinary(t)

	const (
		chainA       environment.ChainID       = "quorum-chain-a"
		chainB       environment.ChainID       = "quorum-chain-b"
		instanceA    environment.IBCInstanceID = "quorum-ibc-a"
		instanceB    environment.IBCInstanceID = "quorum-ibc-b"
		connectionID environment.ConnectionID  = "quorum-a-b"
		clientA      environment.ClientID      = "quorum-client-a"
		clientB      environment.ClientID      = "quorum-client-b"
		attestorA1   environment.AttestorID    = "quorum-attestor-a1"
		attestorA2   environment.AttestorID    = "quorum-attestor-a2"
		attestorB    environment.AttestorID    = "quorum-attestor-b"
		deployer     environment.AuthorityID   = "quorum-deployer"
		signerA1     environment.AuthorityID   = "quorum-signer-a1"
		signerA2     environment.AuthorityID   = "quorum-signer-a2"
		signerB      environment.AuthorityID   = "quorum-signer-b"
	)
	// Distinct, deterministic identities -- not Anvil's provider-default
	// funded accounts, and distinct from testDeployerPrivateKeyHex's suffix
	// so this test's on-chain attestor sets are unambiguous.
	const (
		signerA1Key = "0000000000000000000000000000000000000000000000000000000000000011"
		signerA2Key = "0000000000000000000000000000000000000000000000000000000000000012"
		signerBKey  = "0000000000000000000000000000000000000000000000000000000000000013"
	)

	spec := environment.Spec{
		Chains: []environment.ChainSpec{
			environment.ManagedAnvil{ID: chainA, EVMChainID: 45337},
			environment.ManagedAnvil{ID: chainB, EVMChainID: 45338},
		},
		IBCInstances: []environment.IBCInstanceSpec{
			environment.NewIBCInstance{ID: instanceA, Chain: chainA, Authority: deployer},
			environment.NewIBCInstance{ID: instanceB, Chain: chainB, Authority: deployer},
		},
		Connections: []environment.ConnectionSpec{{
			ID: connectionID,
			A: environment.NewClient{
				ID: clientA, IBCInstance: instanceA, Authority: deployer, MinRequiredSignatures: 2,
			},
			B: environment.NewClient{
				ID: clientB, IBCInstance: instanceB, Authority: deployer, MinRequiredSignatures: 1,
			},
		}},
		// attestorA1/attestorA2 both back clientA (which requires 2 sigs);
		// attestorB alone backs clientB (which requires only 1).
		Attestors: []environment.AttestorSpec{
			{ID: attestorA1, Client: clientA, Authority: signerA1},
			{ID: attestorA2, Client: clientA, Authority: signerA2},
			{ID: attestorB, Client: clientB, Authority: signerB},
		},
	}
	runtime := environment.Runtime{Authorities: map[environment.AuthorityID]environment.EVMAuthority{
		deployer: {PrivateKeyHex: testDeployerPrivateKeyHex},
		signerA1: {PrivateKeyHex: signerA1Key},
		signerA2: {PrivateKeyHex: signerA2Key},
		signerB:  {PrivateKeyHex: signerBKey},
	}}

	env, err := environment.Start(t.Context(), spec, runtime)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, env.Close(context.Background())) })

	connection, err := env.Connection(connectionID)
	require.NoError(t, err)

	resolvedA1, err := env.Attestor(attestorA1)
	require.NoError(t, err)
	resolvedA2, err := env.Attestor(attestorA2)
	require.NoError(t, err)
	resolvedB, err := env.Attestor(attestorB)
	require.NoError(t, err)

	resolvedChainA, err := env.Chain(chainA)
	require.NoError(t, err)
	resolvedChainB, err := env.Chain(chainB)
	require.NoError(t, err)

	resolvedInstanceA, err := env.IBCInstance(instanceA)
	require.NoError(t, err)
	resolvedInstanceB, err := env.IBCInstance(instanceB)
	require.NoError(t, err)

	chainAID := strconv.FormatUint(resolvedChainA.EVMChainID(), 10)
	chainBID := strconv.FormatUint(resolvedChainB.EVMChainID(), 10)

	// buildRelayerDriver writes and migrates a fresh relayer config wired to
	// this same connection/chain pair; includeSecondAttestorA controls
	// whether the config knows about both of clientA's real attestors or
	// just one.
	buildRelayerDriver := func(t *testing.T, includeSecondAttestorA bool) *ibclink.Driver {
		t.Helper()

		dir := t.TempDir()
		signerKeyPath := filepath.Join(dir, "relayer-key.json")
		key, err := crypto.GenerateKey()
		require.NoError(t, err)
		require.NoError(t, keyfile.Store(signerKeyPath, keyfile.ECDSA, crypto.FromECDSA(key)))

		configPath := filepath.Join(dir, "ibc-link.config.yaml")
		driver, err := ibclink.NewDriver(configPath)
		require.NoError(t, err)
		require.NoError(t, env.BindIBCLink(driver))

		chainARPC, err := driver.ChainRPC(string(chainA))
		require.NoError(t, err)
		chainBRPC, err := driver.ChainRPC(string(chainB))
		require.NoError(t, err)

		attestors := []ibclink.RelayerAttestor{
			{Name: string(attestorA1), Type: ibclink.RelayerAttestorRemote, GRPC: resolvedA1.Endpoint()},
			{Name: string(attestorB), Type: ibclink.RelayerAttestorRemote, GRPC: resolvedB.Endpoint()},
		}
		if includeSecondAttestorA {
			attestors = append(attestors, ibclink.RelayerAttestor{
				Name: string(attestorA2), Type: ibclink.RelayerAttestorRemote, GRPC: resolvedA2.Endpoint(),
			})
		}

		cfg := ibclink.RelayerConfig{
			DBPath:        filepath.Join(dir, "relayer.db"),
			SignerAlias:   "relayer-key",
			SignerKeyFile: signerKeyPath,
			Chains: []ibclink.RelayerChain{
				{ChainID: chainAID, RPC: chainARPC, ICS26Router: string(resolvedInstanceA.Locator())},
				{ChainID: chainBID, RPC: chainBRPC, ICS26Router: string(resolvedInstanceB.Locator())},
			},
			Connections: []ibclink.RelayerConnection{{
				ChainA: chainAID, ClientA: string(connection.A().Locator()),
				ChainB: chainBID, ClientB: string(connection.B().Locator()),
			}},
			Routes:    []ibclink.RelayerRoute{{SourceChain: chainAID, SourceClient: string(connection.A().Locator())}},
			Attestors: attestors,
		}

		require.NoError(t, ibclink.WriteRelayerConfig(configPath, cfg))
		require.NoError(t, driver.MigrateUp(t.Context()))
		return driver
	}

	t.Run("insufficientAttestorsFailsStartup", func(t *testing.T) {
		driver := buildRelayerDriver(t, false)

		_, err := driver.StartRelayer(t.Context())

		require.Error(t, err)
		require.ErrorContains(t, err, "on-chain quorum requires 2")
	})

	t.Run("correctlyConfiguredAttestorsStartSuccessfully", func(t *testing.T) {
		driver := buildRelayerDriver(t, true)

		relayer, err := driver.StartRelayer(t.Context())
		require.NoError(t, err)

		stopCtx, cancel := context.WithTimeout(context.Background(), relayerStopTimeout)
		defer cancel()
		require.NoError(t, relayer.Stop(stopCtx))
	})
}
