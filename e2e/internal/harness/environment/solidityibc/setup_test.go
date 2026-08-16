// SPDX-License-Identifier: Apache-2.0

package solidityibc

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/gen/go/solidity-abi/accessmanager"
)

const testMiningTimeout = 30 * time.Second

type failSendBackend struct {
	contractBackend
	failAt int
	sends  int
	hash   common.Hash
}

func (b *failSendBackend) SendTransaction(ctx context.Context, tx *gethtypes.Transaction) error {
	b.sends++
	if b.sends == b.failAt {
		b.hash = tx.Hash()
		return errors.New("injected transaction submission failure")
	}
	return b.contractBackend.SendTransaction(ctx, tx)
}

func TestSetupDeploysAndAttachesSolidityIBCInstanceAndClient(t *testing.T) {
	authority, err := evm.NewAccount()
	require.NoError(t, err)
	attestor, err := evm.NewAccount()
	require.NoError(t, err)
	secondAttestor, err := evm.NewAccount()
	require.NoError(t, err)
	clientAuthority, err := evm.NewAccount()
	require.NoError(t, err)

	backend := simulated.NewBackend(gethtypes.GenesisAlloc{
		authority.Address():       {Balance: ether(100)},
		clientAuthority.Address(): {Balance: ether(100)},
	})
	startMining(t, backend)
	setup, err := newSetup(backend.Client(), big.NewInt(1337), testMiningTimeout)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	instance, err := setup.DeployInstance(ctx, authority)
	require.NoError(t, err)

	attachedInstance, err := setup.AttachInstance(ctx, instance.Router)
	require.NoError(t, err)
	require.Equal(t, instance, attachedInstance)
	grantCustomClientRole(ctx, t, setup, authority, instance, clientAuthority.Address())

	prepared, err := setup.PrepareClient(ctx, clientAuthority, instance.Router, AttestationClientConfig{
		ID:                    "eth-chain-b",
		CounterpartyClientID:  "eth-chain-a",
		Attestors:             []common.Address{attestor.Address(), secondAttestor.Address()},
		MinRequiredSignatures: 2,
		InitialHeight:         1,
		InitialTimestamp:      1_700_000_000,
	})
	require.NoError(t, err)
	client, err := prepared.Deploy(ctx)
	require.NoError(t, err)
	require.Equal(t, "eth-chain-b", client.ID)
	require.Equal(t, "eth-chain-a", client.CounterpartyClientID)
	require.Equal(t, []common.Address{attestor.Address(), secondAttestor.Address()}, client.Attestors)
	require.Equal(t, uint8(2), client.MinRequiredSignatures)

	attachedClient, err := setup.AttachClient(ctx, instance.Router, client.ID, client.CounterpartyClientID)
	require.NoError(t, err)
	require.Equal(t, client, attachedClient)

	_, err = setup.AttachClient(ctx, instance.Router, client.ID, "wrong-counterparty")
	require.ErrorContains(t, err, `counterparty id is "eth-chain-a", want "wrong-counterparty"`)

	// Duplicate registration is rejected by a read-only vacancy check before a
	// second light-client contract is deployed.
	_, err = setup.PrepareClient(ctx, clientAuthority, instance.Router, AttestationClientConfig{
		ID:                    "eth-chain-b",
		CounterpartyClientID:  "eth-chain-a",
		Attestors:             []common.Address{attestor.Address(), secondAttestor.Address()},
		MinRequiredSignatures: 2,
		InitialHeight:         2,
		InitialTimestamp:      1_700_000_001,
	})
	require.ErrorContains(t, err, `Client "eth-chain-b" is already registered`)
}

func TestPrepareClientRejectsInvalidConfigurationBeforeSideEffects(t *testing.T) {
	authority, err := evm.NewAccount()
	require.NoError(t, err)
	backend := simulated.NewBackend(gethtypes.GenesisAlloc{
		authority.Address(): {Balance: ether(1)},
	})
	t.Cleanup(func() { require.NoError(t, backend.Close()) })
	setup, err := newSetup(backend.Client(), big.NewInt(1337), testMiningTimeout)
	require.NoError(t, err)

	prepared, err := setup.PrepareClient(context.Background(), authority, common.Address{}, AttestationClientConfig{
		ID:                    "client-0",
		CounterpartyClientID:  "remote",
		Attestors:             []common.Address{authority.Address()},
		MinRequiredSignatures: 1,
		InitialHeight:         1,
		InitialTimestamp:      1,
	})
	require.ErrorContains(t, err, "not a valid Solidity IBC custom client identifier")
	require.Nil(t, prepared)
}

func TestDeployInstanceReportsBroadcastFailure(t *testing.T) {
	authority, err := evm.NewAccount()
	require.NoError(t, err)
	backend := simulated.NewBackend(gethtypes.GenesisAlloc{
		authority.Address(): {Balance: ether(100)},
	})
	startMining(t, backend)
	failing := &failSendBackend{contractBackend: backend.Client(), failAt: 2}
	setup, err := newSetup(failing, big.NewInt(1337), testMiningTimeout)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	instance, err := setup.DeployInstance(ctx, authority)
	require.Equal(t, Instance{}, instance)
	require.ErrorContains(t, err, "deploy ICS26Router implementation")
	require.ErrorContains(t, err, "injected transaction submission failure")
	require.ErrorContains(t, err, failing.hash.Hex())
}

func TestAwaitMinedTimesOutWhenNeverMined(t *testing.T) {
	authority, err := evm.NewAccount()
	require.NoError(t, err)
	backend := simulated.NewBackend(gethtypes.GenesisAlloc{
		authority.Address(): {Balance: ether(1)},
	})
	t.Cleanup(func() { require.NoError(t, backend.Close()) })
	setup, err := newSetup(backend.Client(), big.NewInt(1337), 200*time.Millisecond)
	require.NoError(t, err)
	tx := gethtypes.NewTx(&gethtypes.LegacyTx{Nonce: 77, Gas: 21_000})

	receipt, err := setup.awaitMined(context.Background(), "unmined confirmation", tx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "unmined confirmation")
	require.Nil(t, receipt)
}

func TestAwaitMinedReturnsCancellation(t *testing.T) {
	authority, err := evm.NewAccount()
	require.NoError(t, err)
	backend := simulated.NewBackend(gethtypes.GenesisAlloc{
		authority.Address(): {Balance: ether(1)},
	})
	t.Cleanup(func() { require.NoError(t, backend.Close()) })
	setup, err := newSetup(backend.Client(), big.NewInt(1337), testMiningTimeout)
	require.NoError(t, err)
	tx := gethtypes.NewTx(&gethtypes.LegacyTx{Nonce: 77, Gas: 21_000})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	receipt, err := setup.awaitMined(ctx, "canceled confirmation", tx)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, receipt)
}

func startMining(t *testing.T, backend *simulated.Backend) {
	t.Helper()
	backend.Commit()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				backend.Commit()
			}
		}
	}()
	t.Cleanup(func() {
		close(stop)
		wg.Wait()
		require.NoError(t, backend.Close())
	})
}

func grantCustomClientRole(
	ctx context.Context,
	t *testing.T,
	setup *Setup,
	admin evm.Account,
	instance Instance,
	account common.Address,
) {
	t.Helper()
	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)
	require.NotNil(t, routerABI)
	selector, err := customAddClientSelector(*routerABI)
	require.NoError(t, err)
	manager, err := accessmanager.NewAccessManager(instance.AccessManager, setup.backend)
	require.NoError(t, err)

	const roleID uint64 = 42
	_, tx, err := setup.send(ctx, admin, func(opts *bind.TransactOpts) (common.Address, *gethtypes.Transaction, error) {
		transaction, sendErr := manager.SetTargetFunctionRole(
			opts,
			instance.Router,
			[][4]byte{selector},
			roleID,
		)
		return common.Address{}, transaction, sendErr
	})
	require.NoError(t, err)
	_, err = setup.awaitMined(ctx, "configure custom Client role", tx)
	require.NoError(t, err)

	_, tx, err = setup.send(ctx, admin, func(opts *bind.TransactOpts) (common.Address, *gethtypes.Transaction, error) {
		transaction, sendErr := manager.GrantRole(opts, roleID, account, uint32(0))
		return common.Address{}, transaction, sendErr
	})
	require.NoError(t, err)
	_, err = setup.awaitMined(ctx, "grant custom Client role", tx)
	require.NoError(t, err)
}

func ether(amount int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(amount), big.NewInt(1_000_000_000_000_000_000))
}
