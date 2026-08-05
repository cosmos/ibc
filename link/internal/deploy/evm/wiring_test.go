package evm

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/internal/deploy"
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

func TestProvisionRegisterDiscoverVerify(t *testing.T) {
	d, _, _ := newSimDriver(t)
	ctx := context.Background()

	core, err := d.ProvisionCore(ctx, deploy.CoreParams{})
	require.NoError(t, err)
	router := core.Router
	require.NotEmpty(t, router)
	require.NotEmpty(t, core.TargetData["accessManager"])
	require.NotEmpty(t, core.TargetData["ics26RouterImplementation"])
	require.Len(t, core.TxHashes, 4)

	// not registered yet
	_, registered, err := d.ClientRegistered(ctx, router, "link-2")
	require.NoError(t, err)
	require.False(t, registered)

	spec := deploy.ClientSpec{
		ClientID:             "link-2",
		Type:                 deploy.ClientTypeAttestation,
		CounterpartyChainID:  "2",
		CounterpartyClientID: "link-1",
		Params: deploy.AttestationParams{
			Attestors:        []string{"0x00000000000000000000000000000000000000aa"},
			Threshold:        1,
			InitialHeight:    5,
			InitialTimestamp: 500,
		},
	}
	ref, err := d.ProvisionClient(ctx, spec)
	require.NoError(t, err)
	require.NotEmpty(t, ref.Address)
	require.NotEmpty(t, ref.TxHash)

	id, err := d.RegisterClient(ctx, router, spec, ref)
	require.NoError(t, err)
	require.Equal(t, "link-2", id)

	got, registered, err := d.ClientRegistered(ctx, router, "link-2")
	require.NoError(t, err)
	require.True(t, registered)
	require.Equal(t, ref.Address, got)

	ok, err := d.HasCode(ctx, router)
	require.NoError(t, err)
	require.True(t, ok)

	m, err := d.Discover(ctx, router)
	require.NoError(t, err)
	require.Equal(t, router, m.Core.Router)
	require.Equal(t, core.TargetData["accessManager"], m.TargetData["accessManager"])
	c, found := m.Client("link-2")
	require.True(t, found)
	require.Equal(t, ref.Address, c.Address)
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

// The PUBLIC_ROLE grant happens inside ProvisionCore and is asserted
// end-to-end by the e2e module's deploy test; this pins the selector set.
func TestPublicRelayingSelectors(t *testing.T) {
	selectors, err := publicRelayingSelectors()
	require.NoError(t, err)
	require.Len(t, selectors, len(publicRelayingMethods))

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)
	require.Equal(t, [4]byte(routerABI.Methods["recvPacket"].ID), selectors[0])
}
