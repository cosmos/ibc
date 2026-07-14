package eureka

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
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/chain/evm"

	gethtypes "github.com/ethereum/go-ethereum/core/types"
)

type failSendBackend struct {
	contractBackend
	failAt int
	sends  int
}

type acceptThenFailBackend struct {
	contractBackend
	accepted common.Hash
}

func (b *acceptThenFailBackend) SendTransaction(ctx context.Context, tx *gethtypes.Transaction) error {
	if err := b.contractBackend.SendTransaction(ctx, tx); err != nil {
		return err
	}
	b.accepted = tx.Hash()
	return errors.New("injected response failure after transaction acceptance")
}

func (b *failSendBackend) SendTransaction(ctx context.Context, tx *gethtypes.Transaction) error {
	b.sends++
	if b.sends == b.failAt {
		return errors.New("injected transaction submission failure")
	}
	return b.contractBackend.SendTransaction(ctx, tx)
}

func TestSetupDeploysAndAttachesEurekaInstanceAndClient(t *testing.T) {
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
	setup, err := newSetup(backend.Client(), big.NewInt(1337))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	instance, instanceReceipts, err := setup.DeployInstance(ctx, authority)
	require.NoError(t, err)
	require.Equal(t, instance.AccessManager, instanceReceipts.AccessManager.ContractAddress)
	require.NotEqual(t, common.Address{}, instanceReceipts.RouterImplementation.ContractAddress)
	require.Equal(t, instance.Router, instanceReceipts.RouterProxy.ContractAddress)
	require.Equal(t, uint64(1), instanceReceipts.AccessManager.Status)
	require.Equal(t, TransactionSubmissionAccepted, instanceReceipts.AccessManager.Submission)
	require.Equal(t, uint64(1), instanceReceipts.RouterImplementation.Status)
	require.Equal(t, uint64(1), instanceReceipts.RouterProxy.Status)

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
	client, clientReceipts, err := prepared.Deploy(ctx)
	require.NoError(t, err)
	require.Equal(t, client.Address, clientReceipts.LightClient.ContractAddress)
	require.Equal(t, uint64(1), clientReceipts.LightClient.Status)
	require.Equal(t, uint64(1), clientReceipts.Registration.Status)
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
	setup, err := newSetup(backend.Client(), big.NewInt(1337))
	require.NoError(t, err)

	prepared, err := setup.PrepareClient(context.Background(), authority, common.Address{}, AttestationClientConfig{
		ID:                    "client-0",
		CounterpartyClientID:  "remote",
		Attestors:             []common.Address{authority.Address()},
		MinRequiredSignatures: 1,
		InitialHeight:         1,
		InitialTimestamp:      1,
	})
	require.ErrorContains(t, err, "not a valid Eureka custom client identifier")
	require.Nil(t, prepared)
}

func TestDeployInstanceReturnsMinedPrefixAndAmbiguousNextTransaction(t *testing.T) {
	authority, err := evm.NewAccount()
	require.NoError(t, err)
	backend := simulated.NewBackend(gethtypes.GenesisAlloc{
		authority.Address(): {Balance: ether(100)},
	})
	startMining(t, backend)
	setup, err := newSetup(&failSendBackend{contractBackend: backend.Client(), failAt: 2}, big.NewInt(1337))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	instance, receipts, err := setup.DeployInstance(ctx, authority)
	require.ErrorContains(t, err, "injected transaction submission failure")
	require.Equal(t, Instance{}, instance)
	require.NotNil(t, receipts.AccessManager)
	require.Equal(t, uint64(1), receipts.AccessManager.Status)
	require.NotNil(t, receipts.RouterImplementation)
	require.Equal(t, TransactionSubmissionAmbiguous, receipts.RouterImplementation.Submission)
	require.NotEqual(t, common.Hash{}, receipts.RouterImplementation.Hash)
	require.NotEqual(t, common.Address{}, receipts.RouterImplementation.PredictedContractAddress)
	require.Equal(t, common.Address{}, receipts.RouterImplementation.ContractAddress)
	require.False(t, receipts.RouterImplementation.Mined)
	require.Nil(t, receipts.RouterProxy)
}

func TestDeployInstancePreservesAmbiguousHashWhenNodeAcceptedTransaction(t *testing.T) {
	authority, err := evm.NewAccount()
	require.NoError(t, err)
	backend := simulated.NewBackend(gethtypes.GenesisAlloc{
		authority.Address(): {Balance: ether(100)},
	})
	t.Cleanup(func() { require.NoError(t, backend.Close()) })
	wrapped := &acceptThenFailBackend{contractBackend: backend.Client()}
	setup, err := newSetup(wrapped, big.NewInt(1337))
	require.NoError(t, err)

	_, receipts, err := setup.DeployInstance(t.Context(), authority)
	require.ErrorContains(t, err, "injected response failure after transaction acceptance")
	require.NotNil(t, receipts.AccessManager)
	require.Equal(t, TransactionSubmissionAmbiguous, receipts.AccessManager.Submission)
	require.Equal(t, wrapped.accepted, receipts.AccessManager.Hash)
	require.NotEqual(t, common.Address{}, receipts.AccessManager.PredictedContractAddress)
	require.Equal(t, common.Address{}, receipts.AccessManager.ContractAddress)
	require.False(t, receipts.AccessManager.Mined)

	backend.Commit()
	mined, err := backend.Client().TransactionReceipt(t.Context(), wrapped.accepted)
	require.NoError(t, err)
	require.Equal(t, wrapped.accepted, mined.TxHash)
	require.Equal(t, gethtypes.ReceiptStatusSuccessful, mined.Status)
	require.Equal(t, receipts.AccessManager.PredictedContractAddress, mined.ContractAddress)
}

func TestAwaitMinedPreservesSubmittedHashWhenCanceled(t *testing.T) {
	authority, err := evm.NewAccount()
	require.NoError(t, err)
	backend := simulated.NewBackend(gethtypes.GenesisAlloc{
		authority.Address(): {Balance: ether(1)},
	})
	t.Cleanup(func() { require.NoError(t, backend.Close()) })
	setup, err := newSetup(backend.Client(), big.NewInt(1337))
	require.NoError(t, err)
	tx := gethtypes.NewTx(&gethtypes.LegacyTx{Nonce: 77, Gas: 21_000})
	predicted := common.HexToAddress("0x1000000000000000000000000000000000000001")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	evidence, err := setup.awaitMined(ctx, "canceled confirmation", tx, predicted)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, tx.Hash(), evidence.Hash)
	require.Equal(t, TransactionSubmissionAccepted, evidence.Submission)
	require.Equal(t, predicted, evidence.PredictedContractAddress)
	require.Equal(t, common.Address{}, evidence.ContractAddress)
	require.False(t, evidence.Mined)
	require.Zero(t, evidence.Status)
}

func startMining(t *testing.T, backend *simulated.Backend) {
	t.Helper()
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
	accessABI, _, err := loadAccessManagerArtifact()
	require.NoError(t, err)
	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)
	require.NotNil(t, routerABI)
	selector, err := customAddClientSelector(*routerABI)
	require.NoError(t, err)
	manager := bind.NewBoundContract(instance.AccessManager, accessABI, setup.backend, setup.backend, setup.backend)

	const roleID uint64 = 42
	_, tx, err := setup.send(ctx, admin, func(opts *bind.TransactOpts) (common.Address, *gethtypes.Transaction, error) {
		transaction, sendErr := manager.Transact(
			opts,
			"setTargetFunctionRole",
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
		transaction, sendErr := manager.Transact(opts, "grantRole", roleID, account, uint32(0))
		return common.Address{}, transaction, sendErr
	})
	require.NoError(t, err)
	_, err = setup.awaitMined(ctx, "grant custom Client role", tx)
	require.NoError(t, err)
}

func ether(amount int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(amount), big.NewInt(1_000_000_000_000_000_000))
}
