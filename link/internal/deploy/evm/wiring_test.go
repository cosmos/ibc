// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/params"
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

	// Geth 1.17.5 activates Bogota on dev chains (simulated.NewBackend uses
	// params.AllDevChainProtocolChanges); under Bogota the gas estimator
	// returns values that OOG on execution. No real network schedules Bogota,
	// so disabling it matches production. ponytail: drop when geth fixes
	// estimate/execute consistency under Bogota.
	conf := *params.AllDevChainProtocolChanges
	conf.BogotaTime = nil
	sim := simulated.NewBackend(types.GenesisAlloc{
		addr: {Balance: new(big.Int).Lsh(big.NewInt(1), 100)},
	}, func(_ *node.Config, ec *ethconfig.Config) {
		ec.Genesis.Config = &conf
	})
	t.Cleanup(func() { _ = sim.Close() })

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

	d := &Driver{chainID: big.NewInt(simChainID), backend: sim.Client(), key: key}
	return d, sim, addr
}

func TestProvisionRegisterVerify(t *testing.T) {
	d, _, _ := newSimDriver(t)
	ctx := context.Background()

	core, err := d.ProvisionCore(ctx, deploy.CoreParams{})
	require.NoError(t, err)
	router := core.Router
	require.NotEmpty(t, router)
	require.NotEmpty(t, core.TargetData["accessManager"])
	require.NotEmpty(t, core.TargetData["ics26RouterImplementation"])

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
	ref, err := d.ProvisionClient(ctx, core.Router, spec)
	require.NoError(t, err)
	require.NotEmpty(t, ref.Address)

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

	m := manifest.New("1337", "evm")
	m.Core.Router = router
	m.TargetData = core.TargetData
	m.UpsertClient(manifest.Client{
		ClientID:             "link-2",
		Type:                 deploy.ClientTypeAttestation,
		Address:              ref.Address,
		CounterpartyClientID: "link-1",
	})
	report, err := d.Verify(ctx, m)
	require.NoError(t, err)
	require.Empty(t, report.Failed())

	// verification catches drift
	broken := *m
	broken.Clients = []manifest.Client{
		{ClientID: "link-2", Address: "0x0000000000000000000000000000000000000123", CounterpartyClientID: "link-1"},
	}
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

// fakeRPCError mimics go-ethereum's rpc.jsonError: a message plus optional
// structured revert data.
type fakeRPCError struct{ msg, data string }

func (e fakeRPCError) Error() string  { return e.msg }
func (e fakeRPCError) ErrorData() any { return e.data }

func TestClassifyGetClientError(t *testing.T) {
	require.Equal(t, clientNotFound,
		classifyGetClientError(fakeRPCError{"execution reverted", clientNotFoundSelector + "aa"}))
	require.Equal(t, otherError,
		classifyGetClientError(fakeRPCError{"execution reverted", "0xdeadbeef"}))
	require.Equal(t, unstructuredRevert, classifyGetClientError(errors.New("execution reverted")))
	require.Equal(t, clientNotFound, classifyGetClientError(errors.New("rpc: IBCClientNotFound()")))
	require.Equal(t, otherError, classifyGetClientError(errors.New("connection refused")))
	require.Equal(t, otherError, classifyGetClientError(nil))
}

// A getClient revert against a contract that is not a router must surface an
// error at the precheck, never read as "client absent" (which would spend
// gas deploying a client whose registration then fails).
func TestClientRegisteredNonRouter(t *testing.T) {
	d, _, _ := newSimDriver(t)
	ctx := context.Background()
	core, err := d.ProvisionCore(ctx, deploy.CoreParams{})
	require.NoError(t, err)

	// the AccessManager is a healthy contract that is not a router
	_, registered, err := d.ClientRegistered(ctx, core.TargetData["accessManager"], "link-x")
	require.Error(t, err)
	require.False(t, registered)
}

func TestProvisionGMPRegister(t *testing.T) {
	d, _, _ := newSimDriver(t)
	ctx := context.Background()

	core, err := d.ProvisionCore(ctx, deploy.CoreParams{})
	require.NoError(t, err)

	ref, err := d.ProvisionGMP(ctx, core.Router, core.TargetData["accessManager"])
	require.NoError(t, err)
	require.NotEmpty(t, ref.Address)
	require.NotEmpty(t, ref.AccountLogic)

	_, registered, err := d.AppRegistered(ctx, core.Router, deploy.GMPPortID)
	require.NoError(t, err)
	require.False(t, registered)

	require.NoError(t, d.RegisterApp(ctx, core.Router, ref.Address, deploy.GMPPortID))

	got, registered, err := d.AppRegistered(ctx, core.Router, deploy.GMPPortID)
	require.NoError(t, err)
	require.True(t, registered)
	require.Equal(t, ref.Address, got)
}

func TestProvisionIFTAndBridge(t *testing.T) {
	d, _, owner := newSimDriver(t)
	ctx := context.Background()

	core, err := d.ProvisionCore(ctx, deploy.CoreParams{})
	require.NoError(t, err)
	gmp, err := d.ProvisionGMP(ctx, core.Router, core.TargetData["accessManager"])
	require.NoError(t, err)

	token, err := d.ProvisionIFT(ctx, gmp.Address, deploy.IFTSpec{Owner: owner.Hex(), Name: "Foo", Symbol: "FOO"})
	require.NoError(t, err)
	require.NotEmpty(t, token.Address)

	_, _, registered, err := d.IFTBridge(ctx, token.Address, "link-2")
	require.NoError(t, err)
	require.False(t, registered)

	ctor, err := d.ProvisionSendCallConstructor(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, ctor)

	require.NoError(t, d.RegisterIFTBridge(ctx, token.Address, deploy.BridgeSpec{
		ClientID:            "link-2",
		CounterpartyIFT:     "0x00000000000000000000000000000000000000cp",
		SendCallConstructor: ctor,
	}))

	cp, gotCtor, registered, err := d.IFTBridge(ctx, token.Address, "link-2")
	require.NoError(t, err)
	require.True(t, registered)
	require.Equal(t, "0x00000000000000000000000000000000000000cp", cp)
	require.Equal(t, ctor, gotCtor)
}

func TestVerifyGMPAndIFT(t *testing.T) {
	d, _, owner := newSimDriver(t)
	ctx := context.Background()

	core, err := d.ProvisionCore(ctx, deploy.CoreParams{})
	require.NoError(t, err)
	gmp, err := d.ProvisionGMP(ctx, core.Router, core.TargetData["accessManager"])
	require.NoError(t, err)
	require.NoError(t, d.RegisterApp(ctx, core.Router, gmp.Address, deploy.GMPPortID))
	token, err := d.ProvisionIFT(ctx, gmp.Address, deploy.IFTSpec{Owner: owner.Hex(), Name: "Foo", Symbol: "FOO"})
	require.NoError(t, err)
	ctor, err := d.ProvisionSendCallConstructor(ctx)
	require.NoError(t, err)
	require.NoError(t, d.RegisterIFTBridge(ctx, token.Address, deploy.BridgeSpec{
		ClientID: "link-2", CounterpartyIFT: "0xcp", SendCallConstructor: ctor,
	}))

	m := manifest.New("1337", "evm")
	m.Core.Router = core.Router
	m.TargetData = core.TargetData
	m.GMP = &manifest.GMP{Address: gmp.Address, AccountLogic: gmp.AccountLogic, Port: deploy.GMPPortID}
	m.UpsertToken(manifest.Token{Symbol: "FOO", Name: "Foo", Address: token.Address, Owner: owner.Hex()})
	require.True(
		t,
		m.UpsertBridge(
			token.Address,
			manifest.Bridge{ClientID: "link-2", CounterpartyIFT: "0xcp", SendCallConstructor: ctor},
		),
	)

	report, err := d.Verify(ctx, m)
	require.NoError(t, err)
	require.Empty(t, report.Failed())

	// drift: wrong gmp address fails
	broken := *m
	broken.GMP = &manifest.GMP{Address: "0x0000000000000000000000000000000000000123", Port: deploy.GMPPortID}
	report, err = d.Verify(ctx, &broken)
	require.NoError(t, err)
	require.NotEmpty(t, report.Failed())
}
