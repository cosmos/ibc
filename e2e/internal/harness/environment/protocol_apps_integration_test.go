package environment_test

import (
	"context"
	"testing"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/accessmanager"
)

func TestStartRealizesAppStackAndDummyClients(t *testing.T) {
	requireDocker(t)

	const (
		chainA       environment.ChainID       = "apps-chain-a"
		chainB       environment.ChainID       = "apps-chain-b"
		instanceA    environment.IBCInstanceID = "apps-ibc-a"
		instanceB    environment.IBCInstanceID = "apps-ibc-b"
		connectionID environment.ConnectionID  = "apps-a-b"
		clientA      environment.ClientID      = "apps-client-a"
		clientB      environment.ClientID      = "apps-client-b"
		deployer     environment.AuthorityID   = "deployer"
	)

	spec := environment.Spec{
		Chains: []environment.ChainSpec{
			environment.ManagedAnvil{ID: chainA, EVMChainID: 44337},
			environment.ManagedAnvil{ID: chainB, EVMChainID: 44338},
		},
		IBCInstances: []environment.IBCInstanceSpec{
			environment.NewIBCInstance{ID: instanceA, Chain: chainA, Authority: deployer},
			environment.NewIBCInstance{ID: instanceB, Chain: chainB, Authority: deployer},
		},
		Connections: []environment.ConnectionSpec{{
			ID: connectionID,
			A: environment.DummyClient{
				ID: clientA, IBCInstance: instanceA, Authority: deployer,
			},
			B: environment.DummyClient{
				ID: clientB, IBCInstance: instanceB, Authority: deployer,
			},
		}},
	}
	runtime := environment.Runtime{Authorities: map[environment.AuthorityID]environment.EVMAuthority{
		deployer: {PrivateKeyHex: testDeployerPrivateKeyHex},
	}}

	env, err := environment.Start(t.Context(), spec, runtime)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, env.Close(context.Background())) })

	resolvedInstanceA, err := env.IBCInstance(instanceA)
	require.NoError(t, err)
	resolvedInstanceB, err := env.IBCInstance(instanceB)
	require.NoError(t, err)
	require.NotEqual(t, common.Address{}, common.HexToAddress(string(resolvedInstanceA.ICS20TransferAddress())))
	require.NotEqual(t, common.Address{}, common.HexToAddress(string(resolvedInstanceA.ICS27GMPAddress())))
	require.NotEqual(t, common.Address{}, common.HexToAddress(string(resolvedInstanceB.ICS20TransferAddress())))
	require.NotEqual(t, common.Address{}, common.HexToAddress(string(resolvedInstanceB.ICS27GMPAddress())))

	for _, instance := range []*environment.IBCInstance{resolvedInstanceA, resolvedInstanceB} {
		evmAccess, evmErr := instance.Chain().EVM()
		require.NoError(t, evmErr)
		routerAddr := common.HexToAddress(string(instance.Locator()))
		accessManagerAddr := common.HexToAddress(string(instance.AccessManagerAddress()))
		ics20Addr := common.HexToAddress(string(instance.ICS20TransferAddress()))
		ics27Addr := common.HexToAddress(string(instance.ICS27GMPAddress()))

		require.NoError(t, evmAccess.UseContractCaller(func(caller bind.ContractCaller) error {
			router, routerErr := ics26router.NewContractCaller(routerAddr, caller)
			if routerErr != nil {
				return routerErr
			}
			transferApp, appErr := router.GetIBCApp(&bind.CallOpts{Context: t.Context()}, "transfer")
			require.NoError(t, appErr)
			require.Equal(t, ics20Addr, transferApp)
			gmpApp, appErr := router.GetIBCApp(&bind.CallOpts{Context: t.Context()}, "gmpport")
			require.NoError(t, appErr)
			require.Equal(t, ics27Addr, gmpApp)

			routerABI, abiErr := ics26router.ContractMetaData.GetAbi()
			require.NoError(t, abiErr)
			require.NotNil(t, routerABI)
			recvPacket, methodOK := routerABI.Methods["recvPacket"]
			require.True(t, methodOK)
			var selector [4]byte
			copy(selector[:], recvPacket.ID)

			manager, managerErr := accessmanager.NewAccessManagerCaller(accessManagerAddr, caller)
			require.NoError(t, managerErr)
			unrelated := common.HexToAddress("0xabcdef0000000000000000000000000000000001")
			permission, callErr := manager.CanCall(
				&bind.CallOpts{Context: t.Context()},
				unrelated,
				routerAddr,
				selector,
			)
			require.NoError(t, callErr)
			require.True(t, permission.Immediate)
			require.EqualValues(t, 0, permission.Delay)
			return nil
		}))
	}

	connection, err := env.Connection(connectionID)
	require.NoError(t, err)
	require.Equal(t, connection.B().Locator(), connection.A().CounterpartyLocator())
	require.Equal(t, connection.A().Locator(), connection.B().CounterpartyLocator())
	require.NotEqual(t, common.Address{}, common.HexToAddress(string(connection.A().LightClientAddress())))
	require.NotEqual(t, common.Address{}, common.HexToAddress(string(connection.B().LightClientAddress())))
	require.Empty(t, connection.A().AttestorAddresses())
	require.Empty(t, connection.B().AttestorAddresses())
	require.EqualValues(t, 0, connection.A().MinRequiredSignatures())
	require.EqualValues(t, 0, connection.B().MinRequiredSignatures())

	byChainA, err := env.IBCInstanceForChain(chainA)
	require.NoError(t, err)
	require.Same(t, resolvedInstanceA, byChainA)

	require.NoError(t, env.Close(t.Context()))
}
