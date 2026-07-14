package eureka

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"slices"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/attestation"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/erc1967proxy"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/link/harness/chain/evm"
)

type contractBackend interface {
	bind.ContractBackend
	bind.DeployBackend
}

// Setup performs Eureka setup transactions on one EVM Chain. It borrows the
// client and never closes it.
type Setup struct {
	backend contractBackend
	chainID *big.Int
}

// NewSetup binds Eureka realization to an already-connected EVM client.
func NewSetup(ctx context.Context, client *ethclient.Client) (*Setup, error) {
	if client == nil {
		return nil, fmt.Errorf("eureka setup: nil EVM client")
	}
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("eureka setup: query chain id: %w", err)
	}
	return newSetup(client, chainID)
}

func newSetup(backend contractBackend, chainID *big.Int) (*Setup, error) {
	if backend == nil {
		return nil, fmt.Errorf("eureka setup: nil contract backend")
	}
	if chainID == nil || chainID.Sign() <= 0 {
		return nil, fmt.Errorf("eureka setup: invalid EVM chain id")
	}
	return &Setup{backend: backend, chainID: new(big.Int).Set(chainID)}, nil
}

// DeployInstance deploys AccessManager, ICS26Router implementation, and an
// initialized ERC1967 proxy in dependency order. Every mined transaction is
// copied into receipts before the next side effect begins.
func (s *Setup) DeployInstance(
	ctx context.Context,
	authority evm.Account,
) (Instance, InstanceReceipts, error) {
	var receipts InstanceReceipts
	accessABI, accessBytecode, err := loadAccessManagerArtifact()
	if err != nil {
		return Instance{}, receipts, fmt.Errorf("eureka deploy Instance: %w", err)
	}
	if authorityErr := validateAuthority(authority); authorityErr != nil {
		return Instance{}, receipts, fmt.Errorf("eureka deploy Instance: %w", authorityErr)
	}

	accessAddress, tx, err := s.send(
		ctx,
		authority,
		func(opts *bind.TransactOpts) (common.Address, *types.Transaction, error) {
			address, transaction, _, deployErr := bind.DeployContract(
				opts,
				accessABI,
				accessBytecode,
				s.backend,
				authority.Address(),
			)
			return address, transaction, deployErr
		},
	)
	receipts.AccessManager = submittedEvidence(tx, err, accessAddress)
	if err != nil {
		return Instance{}, receipts, fmt.Errorf("eureka deploy AccessManager: %w", err)
	}
	accessReceipt, err := s.awaitMined(ctx, "deploy AccessManager", tx, accessAddress)
	receipts.AccessManager = accessReceipt
	if err != nil {
		return Instance{}, receipts, err
	}
	if deploymentErr := requireDeploymentAddress("AccessManager", accessAddress, accessReceipt); deploymentErr != nil {
		return Instance{}, receipts, deploymentErr
	}
	if adminErr := s.requireAdmin(ctx, accessAddress, authority.Address()); adminErr != nil {
		return Instance{}, receipts, fmt.Errorf("eureka verify AccessManager: %w", adminErr)
	}

	routerImplementation, tx, err := s.send(
		ctx,
		authority,
		func(opts *bind.TransactOpts) (common.Address, *types.Transaction, error) {
			address, transaction, _, deployErr := ics26router.DeployContract(opts, s.backend)
			return address, transaction, deployErr
		},
	)
	receipts.RouterImplementation = submittedEvidence(tx, err, routerImplementation)
	if err != nil {
		return Instance{}, receipts, fmt.Errorf("eureka deploy ICS26Router implementation: %w", err)
	}
	implementationReceipt, err := s.awaitMined(
		ctx,
		"deploy ICS26Router implementation",
		tx,
		routerImplementation,
	)
	receipts.RouterImplementation = implementationReceipt
	if err != nil {
		return Instance{}, receipts, err
	}
	if deploymentErr := requireDeploymentAddress(
		"ICS26Router implementation",
		routerImplementation,
		implementationReceipt,
	); deploymentErr != nil {
		return Instance{}, receipts, deploymentErr
	}

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	if err != nil {
		return Instance{}, receipts, fmt.Errorf("eureka encode ICS26Router initialization: %w", err)
	}
	if routerABI == nil {
		return Instance{}, receipts, fmt.Errorf("eureka encode ICS26Router initialization: upstream binding has no ABI")
	}
	initialization, err := routerABI.Pack("initialize", accessAddress)
	if err != nil {
		return Instance{}, receipts, fmt.Errorf("eureka encode ICS26Router initialization: %w", err)
	}

	routerProxy, tx, err := s.send(
		ctx,
		authority,
		func(opts *bind.TransactOpts) (common.Address, *types.Transaction, error) {
			address, transaction, _, deployErr := erc1967proxy.DeployContract(
				opts,
				s.backend,
				routerImplementation,
				initialization,
			)
			return address, transaction, deployErr
		},
	)
	receipts.RouterProxy = submittedEvidence(tx, err, routerProxy)
	if err != nil {
		return Instance{}, receipts, fmt.Errorf("eureka deploy initialized ICS26Router proxy: %w", err)
	}
	proxyReceipt, err := s.awaitMined(ctx, "deploy initialized ICS26Router proxy", tx, routerProxy)
	receipts.RouterProxy = proxyReceipt
	instance := Instance{AccessManager: accessAddress, Router: routerProxy}
	if err != nil {
		return Instance{}, receipts, err
	}
	if err := requireDeploymentAddress("ICS26Router proxy", routerProxy, proxyReceipt); err != nil {
		return Instance{}, receipts, err
	}
	if _, err := s.AttachInstance(ctx, routerProxy); err != nil {
		return Instance{}, receipts, fmt.Errorf("eureka verify deployed Instance: %w", err)
	}
	return instance, receipts, nil
}

// AttachInstance resolves the AccessManager from the router locator and
// verifies that both addresses contain the expected contract interfaces.
func (s *Setup) AttachInstance(
	ctx context.Context,
	routerAddress common.Address,
) (Instance, error) {
	if routerAddress == (common.Address{}) {
		return Instance{}, fmt.Errorf("eureka attach Instance: zero ICS26Router address")
	}
	if err := s.requireCode(ctx, "ICS26Router", routerAddress); err != nil {
		return Instance{}, err
	}
	router, err := ics26router.NewContract(routerAddress, s.backend)
	if err != nil {
		return Instance{}, fmt.Errorf("eureka attach Instance: bind ICS26Router: %w", err)
	}
	authority, err := router.Authority(&bind.CallOpts{Context: ctx})
	if err != nil {
		return Instance{}, fmt.Errorf("eureka attach Instance: query ICS26Router authority: %w", err)
	}
	if authority == (common.Address{}) {
		return Instance{}, fmt.Errorf("eureka attach Instance: ICS26Router has a zero authority")
	}
	if err := s.requireCode(ctx, "AccessManager", authority); err != nil {
		return Instance{}, err
	}
	if err := s.requireAccessManager(ctx, authority); err != nil {
		return Instance{}, fmt.Errorf(
			"eureka attach Instance: authority %s is not an AccessManager: %w",
			authority,
			err,
		)
	}
	return Instance{AccessManager: authority, Router: routerAddress}, nil
}

// PreparedClient is a validated, side-effect-free Client deployment. It keeps
// preparation facts private so deployment cannot bypass the graph-wide
// preflight performed by environment realization.
type PreparedClient struct {
	setup     *Setup
	authority evm.Account
	instance  Instance
	config    AttestationClientConfig
}

// PrepareClient validates one Client deployment without submitting a
// transaction. The returned value owns the snapshotted inputs used by Deploy.
func (s *Setup) PrepareClient(
	ctx context.Context,
	authority evm.Account,
	router common.Address,
	config AttestationClientConfig,
) (*PreparedClient, error) {
	config = config.snapshot()
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("eureka prepare Client: %w", err)
	}
	if err := validateAuthority(authority); err != nil {
		return nil, fmt.Errorf("eureka prepare Client: %w", err)
	}
	instance, err := s.AttachInstance(ctx, router)
	if err != nil {
		return nil, fmt.Errorf("eureka prepare Client: %w", err)
	}
	if err := s.requireCanAddCustomClient(ctx, instance, authority.Address()); err != nil {
		return nil, fmt.Errorf("eureka prepare Client: authority cannot register it: %w", err)
	}
	if err := s.verifyClientVacant(ctx, instance, config.ID); err != nil {
		return nil, fmt.Errorf("eureka prepare Client: %w", err)
	}
	return &PreparedClient{setup: s, authority: authority, instance: instance, config: config}, nil
}

// Deploy submits a prepared Client deployment and returns every transaction
// fact known even when a later stage fails.
func (p *PreparedClient) Deploy(ctx context.Context) (Client, ClientReceipts, error) {
	var receipts ClientReceipts
	if p == nil || p.setup == nil {
		return Client{}, receipts, fmt.Errorf("eureka deploy Client: preparation is required")
	}
	s := p.setup
	authority := p.authority
	instance := p.instance
	config := p.config

	clientAddress, tx, err := s.send(
		ctx,
		authority,
		func(opts *bind.TransactOpts) (common.Address, *types.Transaction, error) {
			address, transaction, _, deployErr := attestation.DeployContract(
				opts,
				s.backend,
				config.Attestors,
				config.MinRequiredSignatures,
				config.InitialHeight,
				config.InitialTimestamp,
				config.RoleManager,
			)
			return address, transaction, deployErr
		},
	)
	receipts.LightClient = submittedEvidence(tx, err, clientAddress)
	if err != nil {
		return Client{}, receipts, fmt.Errorf("eureka deploy attestation Client %q: %w", config.ID, err)
	}
	clientReceipt, err := s.awaitMined(
		ctx,
		"deploy attestation Client "+config.ID,
		tx,
		clientAddress,
	)
	receipts.LightClient = clientReceipt
	if err != nil {
		return Client{}, receipts, err
	}
	if deploymentErr := requireDeploymentAddress(
		"attestation Client "+config.ID,
		clientAddress,
		clientReceipt,
	); deploymentErr != nil {
		return Client{}, receipts, deploymentErr
	}

	router, err := ics26router.NewContract(instance.Router, s.backend)
	if err != nil {
		return Client{}, receipts, fmt.Errorf("eureka register Client %q: bind ICS26Router: %w", config.ID, err)
	}
	_, tx, err = s.send(ctx, authority, func(opts *bind.TransactOpts) (common.Address, *types.Transaction, error) {
		transaction, sendErr := router.AddClient(opts, config.ID, ics26router.IICS02ClientMsgsCounterpartyInfo{
			ClientId:     config.CounterpartyClientID,
			MerklePrefix: [][]byte{{}},
		}, clientAddress)
		return common.Address{}, transaction, sendErr
	})
	receipts.Registration = submittedEvidence(tx, err)
	if err != nil {
		return Client{}, receipts, fmt.Errorf("eureka register Client %q: %w", config.ID, err)
	}
	registrationReceipt, err := s.awaitMined(ctx, "register Client "+config.ID, tx)
	receipts.Registration = registrationReceipt
	if err != nil {
		return Client{}, receipts, err
	}

	client, err := s.verifyClient(ctx, instance, config.ID, clientAddress, config.CounterpartyClientID)
	if err != nil {
		return Client{}, receipts, fmt.Errorf("eureka verify deployed Client %q: %w", config.ID, err)
	}
	return client, receipts, nil
}

// verifyClientVacant proves that a custom Client ID is currently unoccupied.
// An unexpected read failure is not treated as vacancy.
func (s *Setup) verifyClientVacant(ctx context.Context, instance Instance, clientID string) error {
	if !validCustomClientID(clientID) {
		return fmt.Errorf("client id %q is not a valid Eureka custom client identifier", clientID)
	}
	router, err := ics26router.NewContract(instance.Router, s.backend)
	if err != nil {
		return fmt.Errorf("verify Client %q vacancy: bind ICS26Router: %w", clientID, err)
	}
	registered, err := router.GetClient(&bind.CallOpts{Context: ctx}, clientID)
	if err == nil {
		return fmt.Errorf("Client %q is already registered at %s", clientID, registered)
	}
	if isIBCClientNotFound(err) {
		return nil
	}
	return fmt.Errorf("verify Client %q vacancy: query ICS26Router: %w", clientID, err)
}

// AttachClient discovers the light-client address from the router, verifies
// the reciprocal counterparty ID and EVM empty Merkle prefix, and confirms the
// registered contract exposes a valid attestation set.
func (s *Setup) AttachClient(
	ctx context.Context,
	router common.Address,
	clientID string,
	counterpartyClientID string,
) (Client, error) {
	instance, err := s.AttachInstance(ctx, router)
	if err != nil {
		return Client{}, fmt.Errorf("eureka attach Client %q: %w", clientID, err)
	}
	return s.verifyClient(ctx, instance, clientID, common.Address{}, counterpartyClientID)
}

func (s *Setup) verifyClient(
	ctx context.Context,
	instance Instance,
	clientID string,
	expectedAddress common.Address,
	counterpartyClientID string,
) (Client, error) {
	if clientID == "" {
		return Client{}, fmt.Errorf("eureka attach Client: empty client id")
	}
	if counterpartyClientID == "" {
		return Client{}, fmt.Errorf("eureka attach Client %q: empty counterparty client id", clientID)
	}
	router, err := ics26router.NewContract(instance.Router, s.backend)
	if err != nil {
		return Client{}, fmt.Errorf("eureka attach Client %q: bind ICS26Router: %w", clientID, err)
	}
	registered, err := router.GetClient(&bind.CallOpts{Context: ctx}, clientID)
	if err != nil {
		return Client{}, fmt.Errorf("eureka attach Client %q: query router client: %w", clientID, err)
	}
	if expectedAddress != (common.Address{}) && registered != expectedAddress {
		return Client{}, fmt.Errorf(
			"eureka attach Client %q: router has address %s, want %s",
			clientID,
			registered,
			expectedAddress,
		)
	}
	if registered == (common.Address{}) {
		return Client{}, fmt.Errorf("eureka attach Client %q: router returned a zero contract address", clientID)
	}
	if codeErr := s.requireCode(ctx, "attestation Client "+clientID, registered); codeErr != nil {
		return Client{}, codeErr
	}
	counterparty, err := router.GetCounterparty(&bind.CallOpts{Context: ctx}, clientID)
	if err != nil {
		return Client{}, fmt.Errorf("eureka attach Client %q: query counterparty: %w", clientID, err)
	}
	if counterparty.ClientId != counterpartyClientID {
		return Client{}, fmt.Errorf(
			"eureka attach Client %q: counterparty id is %q, want %q",
			clientID,
			counterparty.ClientId,
			counterpartyClientID,
		)
	}
	if len(counterparty.MerklePrefix) != 1 || len(counterparty.MerklePrefix[0]) != 0 {
		return Client{}, fmt.Errorf(
			"eureka attach Client %q: counterparty Merkle prefix is not the EVM empty prefix",
			clientID,
		)
	}

	lightClient, err := attestation.NewContract(registered, s.backend)
	if err != nil {
		return Client{}, fmt.Errorf("eureka attach Client %q: bind attestation contract: %w", clientID, err)
	}
	set, err := lightClient.GetAttestationSet(&bind.CallOpts{Context: ctx})
	if err != nil {
		return Client{}, fmt.Errorf("eureka attach Client %q: query attestation set: %w", clientID, err)
	}
	if len(set.AttestorAddresses) == 0 || set.MinRequiredSigs == 0 ||
		int(set.MinRequiredSigs) > len(set.AttestorAddresses) {
		return Client{}, fmt.Errorf("eureka attach Client %q: invalid attestation set", clientID)
	}
	return Client{
		ID:                    clientID,
		Address:               registered,
		CounterpartyClientID:  counterpartyClientID,
		Attestors:             slices.Clone(set.AttestorAddresses),
		MinRequiredSignatures: set.MinRequiredSigs,
	}, nil
}

func (s *Setup) send(
	ctx context.Context,
	authority evm.Account,
	fn func(*bind.TransactOpts) (common.Address, *types.Transaction, error),
) (common.Address, *types.Transaction, error) {
	opts, err := authority.TransactOpts(s.chainID)
	if err != nil {
		return common.Address{}, nil, err
	}
	opts.Context = ctx
	opts.NoSend = true
	address, tx, err := fn(opts)
	if err != nil {
		return common.Address{}, nil, err
	}
	if tx == nil {
		return common.Address{}, nil, fmt.Errorf("transaction was not constructed")
	}
	if err := s.backend.SendTransaction(ctx, tx); err != nil {
		return address, tx, fmt.Errorf("broadcast transaction %s: %w", tx.Hash(), err)
	}
	return address, tx, nil
}

func (s *Setup) awaitMined(
	ctx context.Context,
	stage string,
	tx *types.Transaction,
	predictedAddress ...common.Address,
) (*TransactionEvidence, error) {
	if tx == nil {
		return nil, fmt.Errorf("eureka %s: transaction was not returned", stage)
	}
	receipt, err := bind.WaitMined(ctx, s.backend, tx)
	if err != nil {
		evidence := signedTransactionEvidence(tx, TransactionSubmissionAccepted)
		setPredictedAddress(evidence, predictedAddress)
		return evidence, fmt.Errorf(
			"eureka %s: wait for transaction %s: %w",
			stage,
			tx.Hash(),
			err,
		)
	}
	mined := minedTransactionEvidence(receipt)
	setPredictedAddress(&mined, predictedAddress)
	if receipt.Status != types.ReceiptStatusSuccessful {
		return &mined, fmt.Errorf("eureka %s: transaction %s reverted", stage, tx.Hash())
	}
	return &mined, nil
}

func submittedEvidence(
	tx *types.Transaction,
	sendErr error,
	predictedAddress ...common.Address,
) *TransactionEvidence {
	status := TransactionSubmissionAccepted
	if sendErr != nil {
		status = TransactionSubmissionAmbiguous
	}
	evidence := signedTransactionEvidence(tx, status)
	setPredictedAddress(evidence, predictedAddress)
	return evidence
}

func setPredictedAddress(evidence *TransactionEvidence, predictedAddress []common.Address) {
	if evidence != nil && len(predictedAddress) != 0 {
		evidence.PredictedContractAddress = predictedAddress[0]
	}
}

func isIBCClientNotFound(err error) bool {
	revertData, ok := ethclient.RevertErrorData(err)
	if !ok || len(revertData) < 4 {
		return false
	}
	selector := crypto.Keccak256([]byte("IBCClientNotFound(string)"))[:4]
	return bytes.Equal(revertData[:4], selector)
}

func (s *Setup) requireCode(ctx context.Context, label string, address common.Address) error {
	code, err := s.backend.CodeAt(ctx, address, nil)
	if err != nil {
		return fmt.Errorf("eureka verify %s %s code: %w", label, address, err)
	}
	if len(code) == 0 {
		return fmt.Errorf("eureka verify %s %s: no contract code", label, address)
	}
	return nil
}

func (s *Setup) requireAdmin(ctx context.Context, accessManager, authority common.Address) error {
	accessABI, _, err := loadAccessManagerArtifact()
	if err != nil {
		return err
	}
	contract := bind.NewBoundContract(accessManager, accessABI, s.backend, nil, nil)
	var output []any
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &output, "hasRole", uint64(0), authority); err != nil {
		return fmt.Errorf("query admin role: %w", err)
	}
	if len(output) != 2 {
		return fmt.Errorf("query admin role returned %d values, want 2", len(output))
	}
	isAdmin := *abi.ConvertType(output[0], new(bool)).(*bool)
	if !isAdmin {
		return fmt.Errorf("address %s is not an AccessManager admin", authority)
	}
	return nil
}

func (s *Setup) requireAccessManager(ctx context.Context, address common.Address) error {
	accessABI, _, err := loadAccessManagerArtifact()
	if err != nil {
		return err
	}
	contract := bind.NewBoundContract(address, accessABI, s.backend, nil, nil)
	var output []any
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &output, "ADMIN_ROLE"); err != nil {
		return fmt.Errorf("query ADMIN_ROLE: %w", err)
	}
	if len(output) != 1 {
		return fmt.Errorf("query ADMIN_ROLE returned %d values, want 1", len(output))
	}
	adminRole := *abi.ConvertType(output[0], new(uint64)).(*uint64)
	if adminRole != 0 {
		return fmt.Errorf("ADMIN_ROLE is %d, want 0", adminRole)
	}
	return nil
}

func (s *Setup) requireCanAddCustomClient(
	ctx context.Context,
	instance Instance,
	authority common.Address,
) error {
	routerABI, err := ics26router.ContractMetaData.GetAbi()
	if err != nil {
		return fmt.Errorf("read ICS26Router ABI: %w", err)
	}
	if routerABI == nil {
		return fmt.Errorf("upstream ICS26Router binding has no ABI")
	}
	selector, err := customAddClientSelector(*routerABI)
	if err != nil {
		return err
	}

	accessABI, _, err := loadAccessManagerArtifact()
	if err != nil {
		return err
	}
	contract := bind.NewBoundContract(instance.AccessManager, accessABI, s.backend, nil, nil)
	var output []any
	if err := contract.Call(
		&bind.CallOpts{Context: ctx},
		&output,
		"canCall",
		authority,
		instance.Router,
		selector,
	); err != nil {
		return fmt.Errorf("query addClient permission: %w", err)
	}
	if len(output) != 2 {
		return fmt.Errorf("query addClient permission returned %d values, want 2", len(output))
	}
	immediate := *abi.ConvertType(output[0], new(bool)).(*bool)
	delay := *abi.ConvertType(output[1], new(uint32)).(*uint32)
	if !immediate {
		return fmt.Errorf("address %s cannot call custom addClient immediately (delay %d)", authority, delay)
	}
	return nil
}

func customAddClientSelector(routerABI abi.ABI) ([4]byte, error) {
	for _, method := range routerABI.Methods {
		if method.RawName == "addClient" && len(method.Inputs) == 3 {
			var selector [4]byte
			copy(selector[:], method.ID)
			return selector, nil
		}
	}
	return [4]byte{}, fmt.Errorf("upstream ICS26Router ABI has no custom addClient overload")
}

func validateAuthority(authority evm.Account) error {
	if authority.Address() == (common.Address{}) {
		return fmt.Errorf("authority is required")
	}
	return nil
}

func requireDeploymentAddress(label string, predicted common.Address, evidence *TransactionEvidence) error {
	if evidence == nil || !evidence.Mined {
		return fmt.Errorf("eureka deploy %s: no mined receipt", label)
	}
	if evidence.ContractAddress != predicted {
		return fmt.Errorf(
			"eureka deploy %s: receipt contract address is %s, predicted %s",
			label,
			evidence.ContractAddress,
			predicted,
		)
	}
	return nil
}
