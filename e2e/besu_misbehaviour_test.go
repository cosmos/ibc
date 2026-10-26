// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"math/big"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/besuerrors"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/besumsgs"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/besuqbft"
	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/cli/besu"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

// besuHeadersMisbehaviour is MisbehaviourType.Headers in IBesuLightClientMsgs.
const besuHeadersMisbehaviour uint8 = 1

// Two headers at one height with different state roots, both sealed by a
// quorum of the source validators, prove a double sign and freeze the client.
func TestBesuQBFTClient_DoubleSignFreeze(t *testing.T) {
	t.Parallel()
	requireBesuMisbehaviourFreeze(t, func(m besuMisbehaviour) {
		height := m.trusted.Height + 1
		honest := m.headers.Sealed(height)
		forged := m.headers.Raw(height)
		forged.Root = crypto.Keccak256Hash([]byte("forged state root"))
		conflicting := m.headers.Seal(forged, honest.Validators, m.headers.Quorum())

		receipt := m.submit(honest, conflicting)
		event := m.event(receipt, "DoubleSign")
		doubleSign, err := m.filterer.ParseDoubleSign(event)
		require.NoError(m.t, err)
		require.Equal(m.t, height, doubleSign.RevisionHeight)
	})
}

// A header sealed with the timestamp of the header below it proves time
// non-monotonicity and freezes the client. The forged timestamp is still
// above the trusted one, so the pair, not either header alone, is the proof.
func TestBesuQBFTClient_TimeNonMonotonicityFreeze(t *testing.T) {
	t.Parallel()
	requireBesuMisbehaviourFreeze(t, func(m besuMisbehaviour) {
		lower := m.headers.Sealed(m.trusted.Height + 1)
		require.Greater(m.t, lower.Timestamp, m.trusted.Timestamp)
		forged := m.headers.Raw(m.trusted.Height + 2)
		forged.Time = lower.Timestamp
		higher := m.headers.Seal(forged, lower.Validators, m.headers.Quorum())

		receipt := m.submit(lower, higher)
		event := m.event(receipt, "TimeNonMonotonicity")
		nonMonotonic, err := m.filterer.ParseTimeNonMonotonicity(event)
		require.NoError(m.t, err)
		require.Equal(m.t, higher.Height, nonMonotonic.Height2)
		require.Equal(m.t, lower.Height, nonMonotonic.Height1)
	})
}

// besuMisbehaviour is the view a misbehavior test needs: the headers of the
// source chain the frozen client tracks, and the client's trusted state that
// the evidence's headers verify against.
type besuMisbehaviour struct {
	t             *testing.T
	ctx           context.Context
	headers       e2etest.BesuHeaders
	destination   *environment.EVM
	sender        e2etest.Signer
	client        *environment.IBCClient
	clientAddress common.Address
	filterer      *besuqbft.ContractFilterer
	trusted       *besu.ParsedHeader
}

// requireBesuMisbehaviourFreeze relays a transfer through the Besu QBFT
// client on B tracking A, lets submit freeze it with evidence, then requires
// the client to reject updates and the next transfer to stay pending.
func requireBesuMisbehaviourFreeze(t *testing.T, submit func(besuMisbehaviour)) {
	t.Helper()
	spec, runtime := e2etest.BesuMesh(e2etest.BesuChains(t, besu.QBFT, besu.QBFT))
	env := e2etest.Start(t, spec, runtime)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSigner(t)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.Deploy(t, env, sender, relayerSigner, route)
	transferApp := e2etest.NewTransfer(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	delivered, err := transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(1_000_000)})
	require.NoError(t, err)
	_, err = e2etest.AwaitState(ctx, relayer, delivered.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED)
	require.NoError(t, err)
	require.NoError(t, delivered.VerifyDelivered(ctx))

	m := besuMisbehaviour{
		t:       t,
		ctx:     ctx,
		headers: e2etest.NewBesuHeaders(t, env, route.Source, besu.QBFT),
		sender:  sender,
	}
	destinationChain, err := env.Chain(route.Destination)
	require.NoError(t, err)
	m.destination, err = destinationChain.EVM()
	require.NoError(t, err)
	m.client = besuClientOn(t, env, route.Source, route.Destination)
	m.clientAddress = common.HexToAddress(string(m.client.LightClientAddress()))
	m.filterer, err = besuqbft.NewContractFilterer(m.clientAddress, nil)
	require.NoError(t, err)

	installed := m.clientState()
	require.False(t, installed.IsFrozen)
	m.trusted = m.headers.Sealed(installed.LatestHeight.RevisionHeight)

	submit(m)

	frozen := m.clientState()
	require.True(t, frozen.IsFrozen)
	require.Equal(t, installed.LatestHeight, frozen.LatestHeight)
	m.requireFrozenRejectsUpdate()

	pending, err := transferApp.Send(ctx, e2etest.TransferRequest{Amount: big.NewInt(2_000_000)})
	require.NoError(t, err)
	require.NoError(t, e2etest.RelayAll(ctx, relayer, pending.PacketTx()))
	require.NoError(t, e2etest.AwaitStable(ctx, relayer, pending.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_PENDING))
	require.NoError(t, pending.VerifyEscrowed(ctx))
	require.NoError(t, pending.VerifyCommitmentPresent(ctx))
	require.NoError(t, pending.VerifyReceiptAbsent(ctx))
	require.NoError(t, pending.VerifyNotMinted(ctx))
}

// besuClientOn is the Besu client on chain host that tracks chain tracked.
func besuClientOn(
	t *testing.T,
	env *environment.Environment,
	tracked, host environment.ChainID,
) *environment.IBCClient {
	t.Helper()
	connection, err := env.Connection(e2etest.MeshConnectionID(tracked, host))
	require.NoError(t, err)
	for _, client := range []*environment.IBCClient{connection.A(), connection.B()} {
		if client.IBCInstance().Chain().ID() == host {
			return client
		}
	}
	require.FailNow(t, "connection has no client on chain", host)
	return nil
}

func (m besuMisbehaviour) clientState() besumsgs.IBesuLightClientMsgsClientState {
	m.t.Helper()
	var state besumsgs.IBesuLightClientMsgsClientState
	err := m.destination.UseContractCaller(func(caller bind.ContractCaller) error {
		var readErr error
		state, readErr = besu.ReadClientState(m.ctx, caller, m.clientAddress)
		return readErr
	})
	require.NoError(m.t, err)
	return state
}

func (m besuMisbehaviour) update(header *besu.ParsedHeader) besumsgs.IBesuLightClientMsgsMsgUpdateClient {
	return besumsgs.IBesuLightClientMsgsMsgUpdateClient{
		HeaderRlp:              header.RLP,
		TrustedHeight:          besumsgs.IICS02ClientMsgsHeight{RevisionHeight: m.trusted.Height},
		ConsensusStatePreimage: m.trusted.ConsensusState(),
	}
}

// submit sends both headers as Headers evidence through the router, which
// anyone may call.
func (m besuMisbehaviour) submit(header1, header2 *besu.ParsedHeader) *types.Receipt {
	m.t.Helper()
	bindings := besumsgs.NewBindings()
	evidence, err := bindings.TryPackHeadersMisbehaviour(besumsgs.IBesuLightClientMsgsMsgHeadersMisbehaviour{
		Update1: m.update(header1),
		Update2: m.update(header2),
	})
	require.NoError(m.t, err)
	message, err := bindings.TryPackSubmitMisbehaviour(besumsgs.IBesuLightClientMsgsMsgSubmitMisbehaviour{
		MisbehaviourType: besuHeadersMisbehaviour,
		Misbehaviour:     evidence[4:], //nolint:misspell // generated binding field
	})
	require.NoError(m.t, err)
	data := m.routerCall("submitMisbehaviour", m.client.ID(), message[4:])
	receipt, err := m.sender.BroadcastTx(m.ctx, m.destination, m.router(), data)
	require.NoError(m.t, err)
	return receipt
}

// event is the client's log in receipt with the named event's topic.
func (m besuMisbehaviour) event(receipt *types.Receipt, name string) types.Log {
	m.t.Helper()
	clientABI, err := besuqbft.ContractMetaData.GetAbi()
	require.NoError(m.t, err)
	topic := clientABI.Events[name].ID
	for _, log := range receipt.Logs {
		if log.Address == m.clientAddress && len(log.Topics) > 0 && log.Topics[0] == topic {
			return *log
		}
	}
	require.FailNow(m.t, "client emitted no event", name)
	return types.Log{}
}

// requireFrozenRejectsUpdate requires the router to revert an update the
// client would accept unfrozen with FrozenClientState.
func (m besuMisbehaviour) requireFrozenRejectsUpdate() {
	m.t.Helper()
	update, err := besu.EncodeUpdateClient(m.update(m.headers.Sealed(m.trusted.Height + 3)))
	require.NoError(m.t, err)
	router := m.router()
	call := ethereum.CallMsg{
		From: m.sender.Address(),
		To:   &router,
		Data: m.routerCall("updateClient", m.client.ID(), update),
	}
	err = m.destination.UseContractCaller(func(caller bind.ContractCaller) error {
		_, callErr := caller.CallContract(m.ctx, call, nil)
		return callErr
	})
	var dataErr rpc.DataError
	require.ErrorAs(m.t, err, &dataErr, "update must revert")
	hexData, ok := dataErr.ErrorData().(string)
	require.True(m.t, ok, "revert data %v", dataErr.ErrorData())
	revert, err := hexutil.Decode(hexData)
	require.NoError(m.t, err)
	require.GreaterOrEqual(m.t, len(revert), 4)
	require.Equal(m.t, besuerrors.BindingsFrozenClientStateErrorID().Bytes()[:4], revert[:4])
}

func (m besuMisbehaviour) router() common.Address {
	return common.HexToAddress(string(m.client.IBCInstance().Locator()))
}

func (m besuMisbehaviour) routerCall(method string, args ...any) []byte {
	m.t.Helper()
	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(m.t, err)
	data, err := routerABI.Pack(method, args...)
	require.NoError(m.t, err)
	return data
}
