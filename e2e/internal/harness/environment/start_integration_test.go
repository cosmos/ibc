package environment_test

import (
	"context"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm/anvil"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"

	chainevm "github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
)

// This deterministic identity is not one of Anvil's provider-default funded accounts.
const testDeployerPrivateKeyHex = "0000000000000000000000000000000000000000000000000000000000000005"

func TestStartRealizesSolidityIBCConnectionAndAttestors(t *testing.T) {
	requireDocker(t)
	requireIBCLinkBinary(t)

	const (
		chainA       environment.ChainID       = "chain-a"
		chainB       environment.ChainID       = "chain-b"
		instanceA    environment.IBCInstanceID = "ibc-a"
		instanceB    environment.IBCInstanceID = "ibc-b"
		connectionID environment.ConnectionID  = "a-b"
		clientA      environment.ClientID      = "client-a"
		clientB      environment.ClientID      = "client-b"
		attestorA    environment.AttestorID    = "attestor-a"
		attestorB    environment.AttestorID    = "attestor-b"
		deployer     environment.AuthorityID   = "deployer"
		signerA      environment.AuthorityID   = "signer-a"
		signerB      environment.AuthorityID   = "signer-b"
	)
	// Attestors only sign; they do not need funded accounts. Keeping their
	// identities distinct makes the on-chain attestation sets unambiguous.
	const (
		signerAKey = "0000000000000000000000000000000000000000000000000000000000000001"
		signerBKey = "0000000000000000000000000000000000000000000000000000000000000002"
	)

	spec := environment.Spec{
		Chains: []environment.ChainSpec{
			environment.ManagedAnvil{ID: chainA, EVMChainID: 41337},
			environment.ManagedAnvil{ID: chainB, EVMChainID: 41338},
		},
		IBCInstances: []environment.IBCInstanceSpec{
			environment.NewIBCInstance{ID: instanceA, Chain: chainA, Authority: deployer},
			environment.NewIBCInstance{ID: instanceB, Chain: chainB, Authority: deployer},
		},
		Connections: []environment.ConnectionSpec{{
			ID: connectionID,
			A: environment.NewClient{
				ID:                    clientA,
				IBCInstance:           instanceA,
				Authority:             deployer,
				MinRequiredSignatures: 1,
			},
			B: environment.NewClient{
				ID:                    clientB,
				IBCInstance:           instanceB,
				Authority:             deployer,
				MinRequiredSignatures: 1,
			},
		}},
		Attestors: []environment.AttestorSpec{
			{ID: attestorA, Client: clientA, Authority: signerA},
			{ID: attestorB, Client: clientB, Authority: signerB},
		},
	}
	runtime := environment.Runtime{Authorities: map[environment.AuthorityID]environment.EVMAuthority{
		deployer: {PrivateKeyHex: testDeployerPrivateKeyHex},
		signerA:  {PrivateKeyHex: signerAKey},
		signerB:  {PrivateKeyHex: signerBKey},
	}}

	env, err := environment.Start(t.Context(), spec, runtime)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, env.Close(context.Background())) })

	resolvedInstanceA, err := env.IBCInstance(instanceA)
	require.NoError(t, err)
	resolvedInstanceB, err := env.IBCInstance(instanceB)
	require.NoError(t, err)
	require.True(t, common.IsHexAddress(string(resolvedInstanceA.Locator())))
	require.True(t, common.IsHexAddress(string(resolvedInstanceB.Locator())))
	require.NotEqual(t, common.Address{}, common.HexToAddress(string(resolvedInstanceA.AccessManagerAddress())))
	require.NotEqual(t, common.Address{}, common.HexToAddress(string(resolvedInstanceB.AccessManagerAddress())))

	connection, err := env.Connection(connectionID)
	require.NoError(t, err)
	require.Equal(t, clientA, connection.A().ID())
	require.Equal(t, clientB, connection.B().ID())
	require.NotEmpty(t, connection.A().Locator())
	require.NotEmpty(t, connection.B().Locator())
	require.Equal(t, connection.B().Locator(), connection.A().CounterpartyLocator())
	require.Equal(t, connection.A().Locator(), connection.B().CounterpartyLocator())
	require.NotEqual(t, common.Address{}, common.HexToAddress(string(connection.A().LightClientAddress())))
	require.NotEqual(t, common.Address{}, common.HexToAddress(string(connection.B().LightClientAddress())))

	serviceA, err := env.Attestor(attestorA)
	require.NoError(t, err)
	serviceB, err := env.Attestor(attestorB)
	require.NoError(t, err)
	require.Same(t, connection.A(), serviceA.IBCClient())
	require.Same(t, resolvedInstanceB, serviceA.ObservedIBCInstance())
	require.Same(t, connection.B(), serviceB.IBCClient())
	require.Same(t, resolvedInstanceA, serviceB.ObservedIBCInstance())
	require.NotEqual(t, serviceA.SignerAddress(), serviceB.SignerAddress())
	require.Equal(t, []environment.EVMAddress{serviceA.SignerAddress()}, connection.A().AttestorAddresses())
	require.Equal(t, []environment.EVMAddress{serviceB.SignerAddress()}, connection.B().AttestorAddresses())
	require.EqualValues(t, 1, connection.A().MinRequiredSignatures())
	require.EqualValues(t, 1, connection.B().MinRequiredSignatures())

	require.NoError(t, env.Close(t.Context()))
}

func TestStartAttachesExistingSolidityIBCResources(t *testing.T) {
	requireDocker(t)
	requireIBCLinkBinary(t)

	const (
		chainAEndpoint environment.EndpointBindingID = "chain-a-rpc"
		chainBEndpoint environment.EndpointBindingID = "chain-b-rpc"
		deployer       environment.AuthorityID       = "deployer"
		signerA        environment.AuthorityID       = "signer-a"
		signerB        environment.AuthorityID       = "signer-b"
		signerC        environment.AuthorityID       = "signer-c"
	)
	const (
		signerAKey = "0000000000000000000000000000000000000000000000000000000000000001"
		signerBKey = "0000000000000000000000000000000000000000000000000000000000000002"
		signerCKey = "0000000000000000000000000000000000000000000000000000000000000003"
	)

	chainA := startOutOfBandAnvil(t, "existing-chain-a", 42337)
	chainB := startOutOfBandAnvil(t, "existing-chain-b", 42338)
	deployerAccount, err := chainevm.AccountFromHex(testDeployerPrivateKeyHex)
	require.NoError(t, err)
	deployerMinimum := new(big.Int).Mul(big.NewInt(100), big.NewInt(1_000_000_000_000_000_000))
	require.NoError(t, chainA.EnsureEOABalance(t.Context(), deployerAccount.Address(), deployerMinimum))
	require.NoError(t, chainB.EnsureEOABalance(t.Context(), deployerAccount.Address(), deployerMinimum))
	chains := []environment.ChainSpec{
		environment.AttachedEVM{
			ID: "chain-a", EVMChainID: 42337, Endpoint: chainAEndpoint,
			Timing: attachedTiming(),
		},
		environment.AttachedEVM{
			ID: "chain-b", EVMChainID: 42338, Endpoint: chainBEndpoint,
			Timing: attachedTiming(),
		},
	}
	endpoints := map[environment.EndpointBindingID]environment.EndpointBinding{
		chainAEndpoint: {RPCURL: chainA.RPCURL()},
		chainBEndpoint: {RPCURL: chainB.RPCURL()},
	}

	created, err := environment.Start(t.Context(), environment.Spec{
		Chains: chains,
		IBCInstances: []environment.IBCInstanceSpec{
			environment.NewIBCInstance{ID: "created-ibc-a", Chain: "chain-a", Authority: deployer},
			environment.NewIBCInstance{ID: "created-ibc-b", Chain: "chain-b", Authority: deployer},
		},
		Connections: []environment.ConnectionSpec{{
			ID: "created-connection",
			A: environment.NewClient{
				ID: "created-client-a", IBCInstance: "created-ibc-a", Authority: deployer, MinRequiredSignatures: 1,
			},
			B: environment.NewClient{
				ID: "created-client-b", IBCInstance: "created-ibc-b", Authority: deployer, MinRequiredSignatures: 1,
			},
		}},
		Attestors: []environment.AttestorSpec{
			{ID: "created-attestor-a", Client: "created-client-a", Authority: signerA},
			{ID: "created-attestor-b", Client: "created-client-b", Authority: signerB},
		},
	}, environment.Runtime{
		Endpoints: endpoints,
		Authorities: map[environment.AuthorityID]environment.EVMAuthority{
			deployer: {PrivateKeyHex: testDeployerPrivateKeyHex},
			signerA:  {PrivateKeyHex: signerAKey},
			signerB:  {PrivateKeyHex: signerBKey},
		},
	})
	require.NoError(t, err)

	createdInstanceA, err := created.IBCInstance("created-ibc-a")
	require.NoError(t, err)
	createdInstanceB, err := created.IBCInstance("created-ibc-b")
	require.NoError(t, err)
	createdConnection, err := created.Connection("created-connection")
	require.NoError(t, err)
	require.NoError(t, created.Close(t.Context()))

	attached, err := environment.Start(t.Context(), environment.Spec{
		Chains: chains,
		IBCInstances: []environment.IBCInstanceSpec{
			environment.ExistingIBCInstance{
				ID: "attached-ibc-a", Chain: "chain-a", Locator: createdInstanceA.Locator(),
			},
			environment.ExistingIBCInstance{
				ID: "attached-ibc-b", Chain: "chain-b", Locator: createdInstanceB.Locator(),
			},
		},
		Connections: []environment.ConnectionSpec{{
			ID: "attached-connection",
			A: environment.ExistingClient{
				ID: "attached-client-a", IBCInstance: "attached-ibc-a", Locator: createdConnection.A().Locator(),
			},
			B: environment.ExistingClient{
				ID: "attached-client-b", IBCInstance: "attached-ibc-b", Locator: createdConnection.B().Locator(),
			},
		}},
		Attestors: []environment.AttestorSpec{
			{ID: "attached-attestor-a", Client: "attached-client-a", Authority: signerA},
			{ID: "attached-attestor-b", Client: "attached-client-b", Authority: signerB},
		},
	}, environment.Runtime{
		Endpoints: endpoints,
		Authorities: map[environment.AuthorityID]environment.EVMAuthority{
			signerA: {PrivateKeyHex: signerAKey},
			signerB: {PrivateKeyHex: signerBKey},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, attached.Close(context.Background())) })

	attachedConnection, err := attached.Connection("attached-connection")
	require.NoError(t, err)
	require.Equal(t, createdConnection.A().Locator(), attachedConnection.A().Locator())
	require.Equal(t, createdConnection.B().Locator(), attachedConnection.B().Locator())
	require.Equal(t, createdConnection.B().Locator(), attachedConnection.A().CounterpartyLocator())
	require.Equal(t, createdConnection.A().Locator(), attachedConnection.B().CounterpartyLocator())

	require.NoError(t, attached.Close(t.Context()))

	// Reuse the original A end against a fresh B router. Keeping the authored
	// Connection, Client, and Instance identities stable reproduces the exact
	// reciprocal locator expected by the existing A Client on the new router.
	mixed, err := environment.Start(t.Context(), environment.Spec{
		Chains: chains,
		IBCInstances: []environment.IBCInstanceSpec{
			environment.ExistingIBCInstance{
				ID: "created-ibc-a", Chain: "chain-a", Locator: createdInstanceA.Locator(),
			},
			environment.NewIBCInstance{
				ID: "created-ibc-b", Chain: "chain-b", Authority: deployer,
			},
		},
		Connections: []environment.ConnectionSpec{{
			ID: "created-connection",
			A: environment.ExistingClient{
				ID: "created-client-a", IBCInstance: "created-ibc-a", Locator: createdConnection.A().Locator(),
			},
			B: environment.NewClient{
				ID:                    "created-client-b",
				IBCInstance:           "created-ibc-b",
				Authority:             deployer,
				MinRequiredSignatures: 1,
			},
		}},
		Attestors: []environment.AttestorSpec{{
			ID: "mixed-attestor-b", Client: "created-client-b", Authority: signerC,
		}},
	}, environment.Runtime{
		Endpoints: endpoints,
		Authorities: map[environment.AuthorityID]environment.EVMAuthority{
			deployer: {PrivateKeyHex: testDeployerPrivateKeyHex},
			signerC:  {PrivateKeyHex: signerCKey},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, mixed.Close(context.Background())) })
	mixedConnection, err := mixed.Connection("created-connection")
	require.NoError(t, err)
	require.Equal(t, createdConnection.A().Locator(), mixedConnection.A().Locator())
	require.Equal(t, createdConnection.B().Locator(), mixedConnection.B().Locator())
	require.NoError(t, mixed.Close(t.Context()))

	_, err = chainA.Height(t.Context())
	require.NoError(t, err, "closing both Environments must leave the attached Chain running")
	_, err = chainB.Height(t.Context())
	require.NoError(t, err, "closing both Environments must leave the attached Chain running")
}

func TestStartRealizesSolidityIBCConnectionAcrossAnvilAndBesu(t *testing.T) {
	requireDocker(t)
	requireIBCLinkBinary(t)

	const (
		deployer environment.AuthorityID = "deployer"
		signerA  environment.AuthorityID = "signer-a"
		signerB  environment.AuthorityID = "signer-b"
	)
	runtime := environment.Runtime{Authorities: map[environment.AuthorityID]environment.EVMAuthority{
		deployer: {PrivateKeyHex: testDeployerPrivateKeyHex},
		signerA:  {PrivateKeyHex: "0000000000000000000000000000000000000000000000000000000000000001"},
		signerB:  {PrivateKeyHex: "0000000000000000000000000000000000000000000000000000000000000002"},
	}}
	spec := environment.Spec{
		Chains: []environment.ChainSpec{
			environment.ManagedAnvil{ID: "anvil", EVMChainID: 43337},
			environment.ManagedBesu{ID: "besu", EVMChainID: 43338},
		},
		IBCInstances: []environment.IBCInstanceSpec{
			environment.NewIBCInstance{ID: "anvil-ibc", Chain: "anvil", Authority: deployer},
			environment.NewIBCInstance{ID: "besu-ibc", Chain: "besu", Authority: deployer},
		},
		Connections: []environment.ConnectionSpec{{
			ID: "anvil-besu",
			A: environment.NewClient{
				ID: "anvil-client", IBCInstance: "anvil-ibc", Authority: deployer, MinRequiredSignatures: 1,
			},
			B: environment.NewClient{
				ID: "besu-client", IBCInstance: "besu-ibc", Authority: deployer, MinRequiredSignatures: 1,
			},
		}},
		Attestors: []environment.AttestorSpec{
			{ID: "anvil-attestor", Client: "anvil-client", Authority: signerA},
			{ID: "besu-attestor", Client: "besu-client", Authority: signerB},
		},
	}

	env, err := environment.Start(t.Context(), spec, runtime)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, env.Close(context.Background())) })

	connection, err := env.Connection("anvil-besu")
	require.NoError(t, err)
	require.Equal(t, connection.B().Locator(), connection.A().CounterpartyLocator())
	require.Equal(t, connection.A().Locator(), connection.B().CounterpartyLocator())
	require.NotEqual(t, common.Address{}, common.HexToAddress(string(connection.A().LightClientAddress())))
	require.NotEqual(t, common.Address{}, common.HexToAddress(string(connection.B().LightClientAddress())))
	require.NoError(t, env.Close(t.Context()))
}

func TestStartMixedManagedAndAttachedEVM(t *testing.T) {
	requireDocker(t)

	const (
		managedID       environment.ChainID           = "managed"
		attachedID      environment.ChainID           = "attached"
		attachedRPC     environment.EndpointBindingID = "attached-rpc"
		managedChainID                                = 31357
		attachedChainID                               = 31358
	)

	outOfBand, err := anvil.Start(t.Context(), anvil.Spec{
		ID:        "out-of-band-attached",
		ChainID:   attachedChainID,
		LogPath:   filepath.Join(t.TempDir(), "out-of-band-anvil.log"),
		StatePath: filepath.Join(t.TempDir(), "out-of-band-anvil-state.json"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, outOfBand.Stop()) })

	spec := environment.Spec{Chains: []environment.ChainSpec{
		environment.ManagedAnvil{
			ID:         managedID,
			EVMChainID: managedChainID,
		},
		environment.AttachedEVM{
			ID:         attachedID,
			EVMChainID: attachedChainID,
			Endpoint:   attachedRPC,
			Timing: environment.Timing{
				CompletionBudget: 30 * time.Second,
				SettleWindow:     time.Second,
				PollInterval:     100 * time.Millisecond,
			},
		},
	}}
	runtime := environment.Runtime{Endpoints: map[environment.EndpointBindingID]environment.EndpointBinding{
		attachedRPC: {RPCURL: outOfBand.RPCURL()},
	}}

	env, err := environment.Start(t.Context(), spec, runtime)
	require.NoError(t, err)

	managed, err := env.Chain(managedID)
	require.NoError(t, err)
	attached, err := env.Chain(attachedID)
	require.NoError(t, err)

	_, err = managed.Height(t.Context())
	require.NoError(t, err, "managed Chain is ready")
	_, err = attached.Height(t.Context())
	require.NoError(t, err, "attached Chain is ready")

	mining, err := managed.Mining()
	require.NoError(t, err)
	require.NotNil(t, mining)
	managedMining := mining
	node, err := managed.NodeLifecycle()
	require.NoError(t, err)
	require.NotNil(t, node)
	managedNode := node

	mining, err = attached.Mining()
	require.Nil(t, mining)
	require.ErrorIs(t, err, environment.ErrCapabilityUnavailable)
	node, err = attached.NodeLifecycle()
	require.Nil(t, node)
	require.ErrorIs(t, err, environment.ErrCapabilityUnavailable)

	funding, err := managed.Funding()
	require.NoError(t, err)
	managedFunding := funding
	target := common.HexToAddress("0x1000000000000000000000000000000000000001")
	minimum := big.NewInt(1_000_000_000_000_000_000)
	require.NoError(t, funding.EnsureEOABalance(t.Context(), target, minimum))
	evmClient, err := managed.EVM()
	require.NoError(t, err)
	balance, err := evmClient.BalanceAt(t.Context(), target, nil)
	require.NoError(t, err)
	require.Equal(t, minimum, balance)
	require.NoError(t, funding.EnsureEOABalance(t.Context(), target, big.NewInt(1)))
	balance, err = evmClient.BalanceAt(t.Context(), target, nil)
	require.NoError(t, err)
	require.Equal(t, minimum, balance, "ensuring a lower minimum must not reduce the balance")
	require.ErrorContains(
		t,
		funding.EnsureEOABalance(t.Context(), common.Address{}, minimum),
		"EOA address is zero",
	)
	requireCodeBearingEOAFundingRejected(t, managed, funding, minimum)

	headerBeforeRestart, err := evmClient.HeaderByNumber(t.Context(), nil)
	require.NoError(t, err)
	require.NoError(t, managedNode.Stop(t.Context()))
	require.NoError(t, managedNode.Start(t.Context()))
	headerAfterRestart, err := evmClient.HeaderByNumber(t.Context(), nil)
	require.NoError(t, err, "an EVM handle created before restart must resolve the replacement client")
	require.GreaterOrEqual(t, headerAfterRestart.Number.Uint64(), headerBeforeRestart.Number.Uint64())

	funding, err = attached.Funding()
	require.Nil(t, funding)
	require.ErrorIs(t, err, environment.ErrCapabilityUnavailable)
	attachedEVM, err := attached.EVM()
	require.NoError(t, err)
	_, err = attachedEVM.HeaderByNumber(t.Context(), nil)
	require.NoError(t, err)

	managedRPC := managed.RPCURL()
	require.NoError(t, env.Close(t.Context()))
	require.NoError(t, env.Close(t.Context()), "Close is idempotent")

	_, err = attachedEVM.HeaderByNumber(t.Context(), nil)
	require.ErrorIs(t, err, environment.ErrEnvironmentClosed)
	_, err = managed.Height(t.Context())
	require.ErrorIs(t, err, environment.ErrEnvironmentClosed)
	require.ErrorIs(t, managedMining.Mine(t.Context(), 1), environment.ErrEnvironmentClosed)
	require.ErrorIs(t, managedNode.Start(t.Context()), environment.ErrEnvironmentClosed)
	require.ErrorIs(
		t,
		managedFunding.EnsureEOABalance(t.Context(), target, minimum),
		environment.ErrEnvironmentClosed,
	)

	assertRPCUnavailable(t, managedRPC)
	_, err = outOfBand.Height(t.Context())
	require.NoError(t, err, "closing the Environment must leave the attached Chain running")
}

func TestStartManagedBesu(t *testing.T) {
	requireDocker(t)

	const chainID environment.ChainID = "besu"
	env, err := environment.Start(t.Context(), environment.Spec{Chains: []environment.ChainSpec{
		environment.ManagedBesu{ID: chainID, EVMChainID: 32357},
	}}, environment.Runtime{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, env.Close(context.Background())) })

	chain, err := env.Chain(chainID)
	require.NoError(t, err)
	_, err = chain.Height(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2*time.Second, chain.Timing().BlockInterval)
	_, err = chain.Mining()
	require.ErrorIs(t, err, environment.ErrCapabilityUnavailable)
	_, err = chain.NodeLifecycle()
	require.ErrorIs(t, err, environment.ErrCapabilityUnavailable)

	funding, err := chain.Funding()
	require.NoError(t, err)
	target := common.HexToAddress("0x2000000000000000000000000000000000000002")
	minimum := big.NewInt(1_000_000_000_000_000_000)
	require.NoError(t, funding.EnsureEOABalance(t.Context(), target, minimum))
	require.NoError(t, funding.EnsureEOABalance(t.Context(), target, new(big.Int).Mul(minimum, big.NewInt(2))))
	evmClient, err := chain.EVM()
	require.NoError(t, err)
	balance, err := evmClient.BalanceAt(t.Context(), target, nil)
	require.NoError(t, err)
	require.Equal(t, new(big.Int).Mul(minimum, big.NewInt(2)), balance)
	require.ErrorContains(
		t,
		funding.EnsureEOABalance(t.Context(), common.Address{}, minimum),
		"EOA address is zero",
	)
	requireCodeBearingEOAFundingRejected(t, chain, funding, minimum)

	rpcURL := chain.RPCURL()
	require.NoError(t, env.Close(t.Context()))
	assertRPCUnavailable(t, rpcURL)
}

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("Docker is unavailable: %v", err)
	}
	if output, err := exec.CommandContext(t.Context(), "docker", "info").CombinedOutput(); err != nil {
		t.Skipf("Docker daemon is unavailable: %v (%s)", err, output)
	}
}

func requireCodeBearingEOAFundingRejected(
	t *testing.T,
	chain *environment.Chain,
	funding *environment.Funding,
	minimum *big.Int,
) {
	t.Helper()

	sender, err := chainevm.NewAccount()
	require.NoError(t, err)
	require.NoError(t, funding.EnsureEOABalance(t.Context(), sender.Address(), minimum))

	evmAccess, err := chain.EVM()
	require.NoError(t, err)
	// The init code returns a single STOP opcode as the runtime contract.
	initCode := common.FromHex("0x6001600c60003960016000f300")
	receipt, err := evmAccess.BroadcastTx(t.Context(), sender, nil, initCode, nil)
	require.NoError(t, err)
	require.NotEqual(t, common.Address{}, receipt.ContractAddress)

	before, err := evmAccess.BalanceAt(t.Context(), receipt.ContractAddress, nil)
	require.NoError(t, err)
	require.ErrorContains(
		t,
		funding.EnsureEOABalance(t.Context(), receipt.ContractAddress, minimum),
		"has contract code and is not an EOA",
	)
	after, err := evmAccess.BalanceAt(t.Context(), receipt.ContractAddress, nil)
	require.NoError(t, err)
	require.Equal(t, before, after, "rejected contract funding must not mutate its balance")
}

func requireIBCLinkBinary(t *testing.T) {
	t.Helper()
	path := ibclink.ResolvedRealBin()
	info, err := os.Stat(path)
	if err != nil {
		if os.Getenv("IBC_BIN") != "" {
			require.NoError(t, err, "explicit IBC_BIN is unavailable at %s", path)
		}
		t.Skipf("IBC Link binary is unavailable at %s: %v; run `make -C link build`", path, err)
	}
	if info.Mode()&0o111 == 0 {
		if os.Getenv("IBC_BIN") != "" {
			t.Fatalf("explicit IBC_BIN is not executable: %s", path)
		}
		t.Skipf("IBC Link binary is not executable: %s; run `make -C link build`", path)
	}
}

func startOutOfBandAnvil(t *testing.T, id string, chainID uint64) *anvil.Chain {
	t.Helper()
	chain, err := anvil.Start(t.Context(), anvil.Spec{
		ID:        id,
		ChainID:   chainID,
		LogPath:   filepath.Join(t.TempDir(), id+".log"),
		StatePath: filepath.Join(t.TempDir(), id+"-state.json"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, chain.Stop()) })
	return chain
}

func attachedTiming() environment.Timing {
	return environment.Timing{
		CompletionBudget: 30 * time.Second,
		SettleWindow:     time.Second,
		PollInterval:     100 * time.Millisecond,
	}
}

func assertRPCUnavailable(t *testing.T, rpcURL string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err == nil {
		defer client.Close()
		_, err = client.BlockNumber(ctx)
	}
	require.Error(t, err, "managed Chain RPC remains reachable after Environment.Close")
}
