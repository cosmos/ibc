package evm

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/attestation"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/erc1967proxy"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/deploy"
	"github.com/cosmos/ibc/link/internal/deploy/evm/accessmanager"
	"github.com/cosmos/ibc/link/internal/deploy/manifest"
)

const simChainID = 1337 // ethclient/simulated fixed chain id

func newSimDriver(t *testing.T) (*Driver, *simulated.Backend, common.Address) {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	addr := crypto.PubkeyToAddress(key.PublicKey)

	sim := simulated.NewBackend(types.GenesisAlloc{
		addr: {Balance: new(big.Int).Lsh(big.NewInt(1), 100)},
	})
	t.Cleanup(func() { sim.Close() })

	// auto-mine so bind.WaitMined returns
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
				sim.Commit()
			}
		}
	}()

	d := newTestDriver(big.NewInt(simChainID), sim.Client(), key)
	return d, sim, addr
}

// deployCoreFixture mirrors what the forge DeployCore script produces.
func deployCoreFixture(t *testing.T, d *Driver, admin common.Address) common.Address {
	t.Helper()
	ctx := context.Background()
	opts, err := d.transactOpts(ctx)
	require.NoError(t, err)

	amAddr, tx, _, err := accessmanager.DeployAccessManager(opts, d.backend, admin)
	require.NoError(t, err)
	require.NoError(t, d.awaitMined(ctx, "am", tx))

	implAddr, tx, _, err := ics26router.DeployContract(opts, d.backend)
	require.NoError(t, err)
	require.NoError(t, d.awaitMined(ctx, "impl", tx))

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)
	init, err := routerABI.Pack("initialize", amAddr)
	require.NoError(t, err)

	proxyAddr, tx, _, err := erc1967proxy.DeployContract(opts, d.backend, implAddr, init)
	require.NoError(t, err)
	require.NoError(t, d.awaitMined(ctx, "proxy", tx))
	return proxyAddr
}

func TestRegisterDiscoverVerify(t *testing.T) {
	d, _, admin := newSimDriver(t)
	ctx := context.Background()
	router := deployCoreFixture(t, d, admin)

	// not registered yet
	_, registered, err := d.ClientRegistered(ctx, router.Hex(), "link-2")
	require.NoError(t, err)
	require.False(t, registered)

	// provision an attestation client via bindings (forge does this in prod)
	opts, err := d.transactOpts(ctx)
	require.NoError(t, err)
	attestor := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	clientAddr, tx, _, err := attestation.DeployContract(opts, d.backend, []common.Address{attestor}, 1, 5, 500, common.Address{})
	require.NoError(t, err)
	require.NoError(t, d.awaitMined(ctx, "client", tx))

	spec := deploy.ClientSpec{
		ClientID:             "link-2",
		Type:                 deploy.ClientTypeAttestation,
		CounterpartyChainID:  "2",
		CounterpartyClientID: "link-1",
	}
	id, err := d.RegisterClient(ctx, router.Hex(), spec, deploy.ClientRef{Address: clientAddr.Hex()})
	require.NoError(t, err)
	require.Equal(t, "link-2", id)

	got, registered, err := d.ClientRegistered(ctx, router.Hex(), "link-2")
	require.NoError(t, err)
	require.True(t, registered)
	require.Equal(t, clientAddr.Hex(), got)

	ok, err := d.HasCode(ctx, router.Hex())
	require.NoError(t, err)
	require.True(t, ok)

	m, err := d.Discover(ctx, router.Hex())
	require.NoError(t, err)
	require.Equal(t, router.Hex(), m.Core.Router)
	require.NotEmpty(t, m.TargetData["accessManager"])
	c, found := m.Client("link-2")
	require.True(t, found)
	require.Equal(t, clientAddr.Hex(), c.Address)
	require.Equal(t, "link-1", c.CounterpartyClientID)

	m.ChainID = "1337"
	report, err := d.Verify(ctx, m)
	require.NoError(t, err)
	require.Empty(t, report.Failed())

	// verification catches drift
	broken := *m
	broken.Clients = []manifest.Client{{ClientID: "link-2", Address: "0x0000000000000000000000000000000000000123", CounterpartyClientID: "link-1"}}
	report, err = d.Verify(ctx, &broken)
	require.NoError(t, err)
	require.NotEmpty(t, report.Failed())
}

func TestOpenPublicRelaying(t *testing.T) {
	d, _, admin := newSimDriver(t)
	ctx := context.Background()
	router := deployCoreFixture(t, d, admin)

	m, err := d.Discover(ctx, router.Hex())
	require.NoError(t, err)
	am := common.HexToAddress(m.TargetData["accessManager"])

	require.NoError(t, d.openPublicRelaying(ctx, am, router))

	// an unrelated address must be able to call recvPacket immediately
	manager, err := accessmanager.NewAccessManager(am, d.backend)
	require.NoError(t, err)
	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)
	var sel [4]byte
	copy(sel[:], routerABI.Methods["recvPacket"].ID)
	perm, err := manager.CanCall(&bind.CallOpts{Context: ctx}, common.HexToAddress("0x1000000000000000000000000000000000000001"), router, sel)
	require.NoError(t, err)
	require.True(t, perm.Immediate)
}
