// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besuibft2"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/besuqbft"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ibcmulticall"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/cli/besu/besutest"
	"github.com/cosmos/ibc/cli/internal/deploy"
	"github.com/cosmos/ibc/cli/internal/deploy/manifest"
	"github.com/cosmos/ibc/gen/go/solidity-abi/accessmanager"
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

	// auto-mine so bind.WaitMined returns. Close only after the miner exits:
	// Commit on a closed backend dereferences a nil beacon.
	stop := make(chan struct{})
	done := make(chan struct{})
	t.Cleanup(func() {
		close(stop)
		<-done
		_ = sim.Close()
	})
	go func() {
		defer close(done)
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
	multicall, err := d.ProvisionMulticall(ctx, core.TargetData["accessManager"])
	require.NoError(t, err)
	require.NotEmpty(t, multicall.Implementation)

	// not registered yet
	_, registered, err := d.ClientRegistered(ctx, router, "cli-2")
	require.NoError(t, err)
	require.False(t, registered)

	spec := deploy.ClientSpec{
		ClientID:             "cli-2",
		CounterpartyChainID:  "2",
		CounterpartyClientID: "cli-1",
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
	require.Equal(t, "cli-2", id)

	got, registered, err := d.ClientRegistered(ctx, router, "cli-2")
	require.NoError(t, err)
	require.True(t, registered)
	require.Equal(t, ref.Address, got)

	ok, err := d.HasCode(ctx, router)
	require.NoError(t, err)
	require.True(t, ok)

	m := manifest.New("1337", "evm")
	m.Core.Router = router
	m.Core.Multicall = multicall.Address
	m.TargetData = core.TargetData
	m.UpsertClient(manifest.Client{
		ClientID:             "cli-2",
		Address:              ref.Address,
		CounterpartyClientID: "cli-1",
	})
	report, err := d.Verify(ctx, m)
	require.NoError(t, err)
	require.Empty(t, report.Failed())

	t.Run("relay permissions", func(t *testing.T) {
		am, accessErr := accessmanager.NewAccessManager(
			common.HexToAddress(core.TargetData["accessManager"]), d.backend,
		)
		require.NoError(t, accessErr)
		multicallABI, accessErr := ibcmulticall.ContractMetaData.GetAbi()
		require.NoError(t, accessErr)
		routerABI, accessErr := ics26router.ContractMetaData.GetAbi()
		require.NoError(t, accessErr)
		caller := common.HexToAddress("0x1234")
		multicallAddress := common.HexToAddress(multicall.Address)
		routerAddress := common.HexToAddress(router)
		opts := &bind.CallOpts{Context: ctx}

		permission, accessErr := am.CanCall(
			opts, caller, multicallAddress, [4]byte(multicallABI.Methods["multicall"].ID),
		)
		require.NoError(t, accessErr)
		require.True(t, permission.Immediate, "multicall must be publicly callable")

		permission, accessErr = am.CanCall(
			opts, multicallAddress, routerAddress, [4]byte(routerABI.Methods["recvPacket"].ID),
		)
		require.NoError(t, accessErr)
		require.True(t, permission.Immediate, "multicall must be allowed to relay packets")

		permission, accessErr = am.CanCall(
			opts, caller, multicallAddress, [4]byte(multicallABI.Methods["upgradeToAndCall"].ID),
		)
		require.NoError(t, accessErr)
		require.False(t, permission.Immediate, "multicall upgrades must remain restricted")

		permission, accessErr = am.CanCall(
			opts, multicallAddress, routerAddress, [4]byte(routerABI.Methods["addClient"].ID),
		)
		require.NoError(t, accessErr)
		require.False(t, permission.Immediate, "multicall must not be allowed to register clients")
	})

	t.Run("missing multicall", func(t *testing.T) {
		broken := *m
		broken.Core.Multicall = ""
		broken.Clients = []manifest.Client{m.Clients[0]}
		broken.Clients[0].Address = common.HexToAddress("0x1234").Hex()
		result, verifyErr := d.Verify(ctx, &broken)
		require.NoError(t, verifyErr)
		require.Len(t, result.Failed(), 2)
		require.Equal(t, "client cli-2 address matches", result.Failed()[0].Name)
		require.Contains(t, result.Failed(), deploy.Check{
			Name: "core multicall recorded", Status: deploy.CheckFailed, Detail: "manifest has no multicall",
		})
	})

	// verification catches drift
	broken := *m
	broken.Clients = []manifest.Client{
		{ClientID: "cli-2", Address: "0x0000000000000000000000000000000000000123", CounterpartyClientID: "cli-1"},
	}
	report, err = d.Verify(ctx, &broken)
	require.NoError(t, err)
	require.NotEmpty(t, report.Failed())
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
	_, registered, err := d.ClientRegistered(ctx, core.TargetData["accessManager"], "cli-x")
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

	_, _, registered, err := d.IFTBridge(ctx, token.Address, "cli-2")
	require.NoError(t, err)
	require.False(t, registered)

	ctor, err := d.ProvisionSendCallConstructor(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, ctor)

	require.NoError(t, d.RegisterIFTBridge(ctx, token.Address, deploy.BridgeSpec{
		ClientID:            "cli-2",
		CounterpartyIFT:     "0x00000000000000000000000000000000000000cp",
		SendCallConstructor: ctor,
	}))

	cp, gotCtor, registered, err := d.IFTBridge(ctx, token.Address, "cli-2")
	require.NoError(t, err)
	require.True(t, registered)
	require.Equal(t, "0x00000000000000000000000000000000000000cp", cp)
	require.Equal(t, ctor, gotCtor)

	limit, err := d.IFTRateLimit(ctx, token.Address)
	require.NoError(t, err)
	require.Zero(t, limit.Capacity.Sign())

	want := deploy.IFTRateLimit{Capacity: big.NewInt(1000), Window: 3600}
	require.NoError(t, d.SetIFTRateLimit(ctx, token.Address, want))
	limit, err = d.IFTRateLimit(ctx, token.Address)
	require.NoError(t, err)
	require.True(t, want.Equal(limit))
}

func TestVerifyGMPAndIFT(t *testing.T) {
	d, _, owner := newSimDriver(t)
	ctx := context.Background()

	core, err := d.ProvisionCore(ctx, deploy.CoreParams{})
	require.NoError(t, err)
	multicall, err := d.ProvisionMulticall(ctx, core.TargetData["accessManager"])
	require.NoError(t, err)
	gmp, err := d.ProvisionGMP(ctx, core.Router, core.TargetData["accessManager"])
	require.NoError(t, err)
	require.NoError(t, d.RegisterApp(ctx, core.Router, gmp.Address, deploy.GMPPortID))
	token, err := d.ProvisionIFT(ctx, gmp.Address, deploy.IFTSpec{Owner: owner.Hex(), Name: "Foo", Symbol: "FOO"})
	require.NoError(t, err)
	ctor, err := d.ProvisionSendCallConstructor(ctx)
	require.NoError(t, err)
	require.NoError(t, d.RegisterIFTBridge(ctx, token.Address, deploy.BridgeSpec{
		ClientID: "cli-2", CounterpartyIFT: "0xcp", SendCallConstructor: ctor,
	}))

	m := manifest.New("1337", "evm")
	m.Core.Router = core.Router
	m.Core.Multicall = multicall.Address
	m.TargetData = core.TargetData
	m.GMP = &manifest.GMP{Address: gmp.Address, AccountLogic: gmp.AccountLogic, Port: deploy.GMPPortID}
	m.UpsertToken(manifest.Token{Symbol: "FOO", Name: "Foo", Address: token.Address, Owner: owner.Hex()})
	require.True(
		t,
		m.UpsertBridge(
			token.Address,
			manifest.Bridge{ClientID: "cli-2", CounterpartyIFT: "0xcp", SendCallConstructor: ctor},
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

// A Besu client deploys the contract for its consensus from recorded params
// alone, registers like any other client, and exposes its state through the
// reads the prover relies on.
func TestProvisionRegisterVerifyBesu(t *testing.T) {
	for consensus, metadata := range map[besu.Consensus]*bind.MetaData{
		besu.QBFT:  besuqbft.ContractMetaData,
		besu.IBFT2: besuibft2.ContractMetaData,
	} {
		t.Run(string(consensus), func(t *testing.T) {
			d, sim, _ := newSimDriver(t)
			ctx := context.Background()
			fixture := besutest.MustFixture(t, consensus)

			core, err := d.ProvisionCore(ctx, deploy.CoreParams{})
			require.NoError(t, err)
			multicall, err := d.ProvisionMulticall(ctx, core.TargetData["accessManager"])
			require.NoError(t, err)

			// the constructor rejects an expired initial state, so trust the
			// fixture's state at the current head time
			head, err := sim.Client().HeaderByNumber(ctx, nil)
			require.NoError(t, err)
			consensusState := fixture.InitialConsensusState()
			consensusState.Timestamp = head.Time

			params := deploy.BesuParams{
				Consensus:             consensus,
				IBCRouter:             fixture.RouterAddress,
				InitialHeight:         fixture.InitialTrustedHeight,
				InitialConsensusState: consensusState,
				TrustingPeriod:        fixture.TrustingPeriod,
				MaxClockDrift:         fixture.MaxClockDrift,
				TrustLevel:            besumsgs.IBesuLightClientMsgsTrustThreshold{Numerator: 1, Denominator: 2},
			}
			spec := deploy.ClientSpec{
				ClientID:             "besu-2",
				CounterpartyChainID:  "2",
				CounterpartyClientID: "besu-1",
				Params:               params,
			}
			ref, err := d.ProvisionClient(ctx, core.Router, spec)
			require.NoError(t, err)

			// the deployed runtime code is embedded in the consensus's creation code
			code, err := sim.Client().CodeAt(ctx, common.HexToAddress(ref.Address), nil)
			require.NoError(t, err)
			require.NotEmpty(t, code)
			require.Contains(t, metadata.Bin, hex.EncodeToString(code))

			id, err := d.RegisterClient(ctx, core.Router, spec, ref)
			require.NoError(t, err)
			require.Equal(t, "besu-2", id)

			m := manifest.New("1337", "evm")
			m.Core.Router = core.Router
			m.Core.Multicall = multicall.Address
			m.TargetData = core.TargetData
			m.UpsertClient(manifest.Client{
				ClientID: "besu-2", Address: ref.Address, CounterpartyClientID: "besu-1",
			})
			report, err := d.Verify(ctx, m)
			require.NoError(t, err)
			require.Empty(t, report.Failed())

			state, err := besu.ReadClientState(ctx, sim.Client(), common.HexToAddress(ref.Address))
			require.NoError(t, err)
			require.Equal(t, besumsgs.IBesuLightClientMsgsClientState{
				IbcRouter:      fixture.RouterAddress,
				LatestHeight:   besumsgs.IICS02ClientMsgsHeight{RevisionHeight: fixture.InitialTrustedHeight},
				TrustingPeriod: fixture.TrustingPeriod,
				MaxClockDrift:  fixture.MaxClockDrift,
				TrustLevel:     params.TrustLevel,
			}, state)

			lightClient, err := besuqbft.NewContractCaller(common.HexToAddress(ref.Address), sim.Client())
			require.NoError(t, err)
			hash, err := lightClient.GetConsensusStateHash(&bind.CallOpts{Context: ctx}, fixture.InitialTrustedHeight)
			require.NoError(t, err)

			want, err := besutest.HashConsensusState(consensusState)
			require.NoError(t, err)
			require.Equal(t, want, common.Hash(hash))
		})
	}
}
