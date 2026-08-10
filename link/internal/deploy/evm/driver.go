// Package evm implements the deploy Target for EVM chains. All contract
// creation bytecode ships inside the binary (go-abigen artifacts plus the
// embedded AccessManager artifact), so deployment needs only an RPC endpoint.
package evm

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"strings"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/attestation"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/erc1967proxy"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/evmiftsendcall"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics27account"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics27gmp"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ift"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/link/internal/deploy"

	_ "embed"
)

// accessManagerJSON is the OpenZeppelin AccessManager v5.6.1 foundry
// artifact (abi + creation bytecode; solc 0.8.28, optimizer 200 runs). To
// upgrade: forge build an OpenZeppelin checkout and extract
// {abi, bytecode: {object}} from out/AccessManager.sol/AccessManager.json.
//
//go:embed artifacts/access_manager.json
var accessManagerJSON []byte

// backend is the subset of ethclient the driver needs; narrowed for tests.
type backend interface {
	bind.ContractBackend
	bind.DeployBackend
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
}

var _ deploy.Target = (*Driver)(nil)

// Driver implements deploy.Target for EVM chains.
type Driver struct {
	chainID *big.Int
	key     *ecdsa.PrivateKey
	backend backend
}

// Options configures an EVM driver.
type Options struct {
	ChainID        string
	RPCURL         string
	DeployerKeyHex string
}

// New connects to the chain and validates its ID. Empty DeployerKeyHex
// builds a read-only driver: queries only, no provisioning or wiring.
func New(ctx context.Context, opts Options) (*Driver, error) {
	var key *ecdsa.PrivateKey
	if opts.DeployerKeyHex != "" {
		var err error
		key, err = crypto.HexToECDSA(strings.TrimPrefix(opts.DeployerKeyHex, "0x"))
		if err != nil {
			return nil, fmt.Errorf("deployer key: %w", err)
		}
	}
	client, err := ethclient.DialContext(ctx, opts.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", opts.RPCURL, err)
	}
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("query chain id: %w", err)
	}
	if chainID.String() != opts.ChainID {
		return nil, fmt.Errorf("rpc %s reports chain id %s, config says %s", opts.RPCURL, chainID, opts.ChainID)
	}
	return &Driver{chainID: chainID, key: key, backend: client}, nil
}

func (d *Driver) SupportedClientTypes() []string {
	return []string{deploy.ClientTypeAttestation}
}

// requireSigner errors if called on a driver built without a deployer
// signer; every mutating operation must call it first.
func (d *Driver) requireSigner() error {
	if d.key == nil {
		return fmt.Errorf("no deployer signer configured for this chain: set chains[].deployer or pass --deployer")
	}
	return nil
}

// accessManagerArtifact parses the embedded AccessManager artifact into its
// ABI and creation bytecode.
func accessManagerArtifact() (abi.ABI, []byte, error) {
	var artifact struct {
		ABI      json.RawMessage `json:"abi"`
		Bytecode struct {
			Object string `json:"object"`
		} `json:"bytecode"`
	}
	if err := json.Unmarshal(accessManagerJSON, &artifact); err != nil {
		return abi.ABI{}, nil, fmt.Errorf("parse access manager artifact: %w", err)
	}
	parsed, err := abi.JSON(bytes.NewReader(artifact.ABI))
	if err != nil {
		return abi.ABI{}, nil, fmt.Errorf("parse access manager abi: %w", err)
	}
	return parsed, common.FromHex(artifact.Bytecode.Object), nil
}

// deployProxy deploys an ERC1967 proxy for impl, initialized by packing
// initialize(initArgs...) from meta's ABI. label names it in logs/errors.
func (d *Driver) deployProxy(
	ctx context.Context, opts *bind.TransactOpts, label string,
	meta *bind.MetaData, impl common.Address, initArgs ...any,
) (common.Address, error) {
	parsed, err := meta.GetAbi()
	if err != nil {
		return common.Address{}, err
	}
	init, err := parsed.Pack("initialize", initArgs...)
	if err != nil {
		return common.Address{}, err
	}
	addr, tx, _, err := erc1967proxy.DeployContract(opts, d.backend, impl, init)
	if err != nil {
		return common.Address{}, fmt.Errorf("deploy %s proxy: %w", label, err)
	}
	return addr, d.awaitMined(ctx, "deploy "+label+" proxy", tx)
}

// ProvisionCore deploys AccessManager + ICS26Router (implementation behind
// an initialized ERC1967 proxy), then binds the relaying selectors to
// PUBLIC_ROLE so any relayer EOA can submit packets. The deployer key stays
// the AccessManager admin; role hardening is a follow-up.
func (d *Driver) ProvisionCore(ctx context.Context, _ deploy.CoreParams) (deploy.CoreRef, error) {
	opts, err := d.transactOpts(ctx)
	if err != nil {
		return deploy.CoreRef{}, err
	}
	selectors, err := publicRelayingSelectors()
	if err != nil {
		return deploy.CoreRef{}, err
	}
	amABI, amBin, err := accessManagerArtifact()
	if err != nil {
		return deploy.CoreRef{}, err
	}

	amAddr, amTx, am, err := bind.DeployContract(opts, amABI, amBin, d.backend, crypto.PubkeyToAddress(d.key.PublicKey))
	if err != nil {
		return deploy.CoreRef{}, fmt.Errorf("deploy AccessManager: %w", err)
	}
	if mineErr := d.awaitMined(ctx, "deploy AccessManager", amTx); mineErr != nil {
		return deploy.CoreRef{}, mineErr
	}

	implAddr, implTx, _, err := ics26router.DeployContract(opts, d.backend)
	if err != nil {
		return deploy.CoreRef{}, fmt.Errorf("deploy ICS26Router implementation: %w", err)
	}
	if mineErr := d.awaitMined(ctx, "deploy ICS26Router implementation", implTx); mineErr != nil {
		return deploy.CoreRef{}, mineErr
	}

	routerAddr, err := d.deployProxy(ctx, opts, "ICS26Router", ics26router.ContractMetaData, implAddr, amAddr)
	if err != nil {
		return deploy.CoreRef{}, err
	}

	// OZ AccessManager PUBLIC_ROLE is type(uint64).max
	roleTx, err := am.Transact(opts, "setTargetFunctionRole", routerAddr, selectors, uint64(math.MaxUint64))
	if err != nil {
		return deploy.CoreRef{}, fmt.Errorf("setTargetFunctionRole: %w", err)
	}
	if mineErr := d.awaitMined(ctx, "setTargetFunctionRole", roleTx); mineErr != nil {
		return deploy.CoreRef{}, mineErr
	}

	return deploy.CoreRef{
		Router: routerAddr.Hex(),
		TargetData: map[string]string{
			"accessManager":             amAddr.Hex(),
			"ics26RouterImplementation": implAddr.Hex(),
		},
	}, nil
}

// ProvisionClient deploys a light client contract.
func (d *Driver) ProvisionClient(ctx context.Context, router string, spec deploy.ClientSpec) (deploy.ClientRef, error) {
	if err := d.requireSigner(); err != nil {
		return deploy.ClientRef{}, err
	}
	if spec.Type != deploy.ClientTypeAttestation {
		return deploy.ClientRef{}, fmt.Errorf(
			"client type %q not supported (supported: %v)",
			spec.Type,
			d.SupportedClientTypes(),
		)
	}
	params, ok := spec.Params.(deploy.AttestationParams)
	if !ok {
		return deploy.ClientRef{}, fmt.Errorf("client %q: params must be deploy.AttestationParams", spec.ClientID)
	}
	attestors, err := attestationArgs(params)
	if err != nil {
		return deploy.ClientRef{}, fmt.Errorf("client %q: %w", spec.ClientID, err)
	}
	opts, err := d.transactOpts(ctx)
	if err != nil {
		return deploy.ClientRef{}, err
	}
	addr, tx, _, err := attestation.DeployContract(
		opts,
		d.backend,
		attestors,
		params.Threshold,
		params.InitialHeight,
		params.InitialTimestamp,
		common.HexToAddress(router),
	)
	if err != nil {
		return deploy.ClientRef{}, fmt.Errorf("deploy attestation client: %w", err)
	}
	if err := d.awaitMined(ctx, "deploy attestation client", tx); err != nil {
		return deploy.ClientRef{}, err
	}
	return deploy.ClientRef{Address: addr.Hex()}, nil
}

// attestationArgs validates attestation params and converts the attestor
// addresses for the contract constructor.
func attestationArgs(p deploy.AttestationParams) ([]common.Address, error) {
	if len(p.Attestors) == 0 {
		return nil, fmt.Errorf("attestors required")
	}
	attestors := make([]common.Address, len(p.Attestors))
	for i, a := range p.Attestors {
		if !common.IsHexAddress(a) {
			return nil, fmt.Errorf("invalid attestor address %q", a)
		}
		attestors[i] = common.HexToAddress(a)
	}
	if p.Threshold == 0 || int(p.Threshold) > len(p.Attestors) {
		return nil, fmt.Errorf("threshold %d invalid for %d attestors", p.Threshold, len(p.Attestors))
	}
	if p.InitialHeight == 0 || p.InitialTimestamp == 0 {
		return nil, fmt.Errorf("initial height and timestamp required")
	}
	return attestors, nil
}

// ProvisionGMP deploys the ICS27Account logic impl, the ICS27GMP impl, and an
// ERC1967 proxy initialized with the router, account logic, and AccessManager
// authority.
func (d *Driver) ProvisionGMP(ctx context.Context, router, accessManager string) (deploy.GMPRef, error) {
	opts, err := d.transactOpts(ctx)
	if err != nil {
		return deploy.GMPRef{}, err
	}
	logicAddr, logicTx, _, err := ics27account.DeployContract(opts, d.backend)
	if err != nil {
		return deploy.GMPRef{}, fmt.Errorf("deploy ICS27Account logic: %w", err)
	}
	if mineErr := d.awaitMined(ctx, "deploy ICS27Account logic", logicTx); mineErr != nil {
		return deploy.GMPRef{}, mineErr
	}
	implAddr, implTx, _, err := ics27gmp.DeployContract(opts, d.backend)
	if err != nil {
		return deploy.GMPRef{}, fmt.Errorf("deploy ICS27GMP implementation: %w", err)
	}
	if mineErr := d.awaitMined(ctx, "deploy ICS27GMP implementation", implTx); mineErr != nil {
		return deploy.GMPRef{}, mineErr
	}
	proxyAddr, err := d.deployProxy(ctx, opts, "ICS27GMP", ics27gmp.ContractMetaData, implAddr,
		common.HexToAddress(router), logicAddr, common.HexToAddress(accessManager))
	if err != nil {
		return deploy.GMPRef{}, err
	}
	return deploy.GMPRef{Address: proxyAddr.Hex(), AccountLogic: logicAddr.Hex()}, nil
}

// ProvisionIFT deploys an IFT impl and an ERC1967 proxy initialized with the
// owner, name, symbol, and GMP address.
func (d *Driver) ProvisionIFT(ctx context.Context, gmp string, spec deploy.IFTSpec) (deploy.IFTRef, error) {
	if spec.Name == "" || spec.Symbol == "" {
		return deploy.IFTRef{}, fmt.Errorf("ift name and symbol required")
	}
	if !common.IsHexAddress(spec.Owner) {
		return deploy.IFTRef{}, fmt.Errorf("invalid owner address %q", spec.Owner)
	}
	opts, err := d.transactOpts(ctx)
	if err != nil {
		return deploy.IFTRef{}, err
	}
	implAddr, implTx, _, err := ift.DeployContract(opts, d.backend)
	if err != nil {
		return deploy.IFTRef{}, fmt.Errorf("deploy IFT implementation: %w", err)
	}
	if mineErr := d.awaitMined(ctx, "deploy IFT implementation", implTx); mineErr != nil {
		return deploy.IFTRef{}, mineErr
	}
	proxyAddr, err := d.deployProxy(ctx, opts, "IFT", ift.ContractMetaData, implAddr,
		common.HexToAddress(spec.Owner), spec.Name, spec.Symbol, common.HexToAddress(gmp))
	if err != nil {
		return deploy.IFTRef{}, err
	}
	return deploy.IFTRef{Address: proxyAddr.Hex()}, nil
}

// ProvisionSendCallConstructor deploys the stateless EVM IFT send-call
// constructor.
func (d *Driver) ProvisionSendCallConstructor(ctx context.Context) (string, error) {
	opts, err := d.transactOpts(ctx)
	if err != nil {
		return "", err
	}
	addr, tx, _, err := evmiftsendcall.DeployContract(opts, d.backend)
	if err != nil {
		return "", fmt.Errorf("deploy EVMIFTSendCallConstructor: %w", err)
	}
	if mineErr := d.awaitMined(ctx, "deploy EVMIFTSendCallConstructor", tx); mineErr != nil {
		return "", mineErr
	}
	return addr.Hex(), nil
}

// transactOpts builds signed transact opts for deployment and wiring
// transactions.
func (d *Driver) transactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	if err := d.requireSigner(); err != nil {
		return nil, err
	}
	opts, err := bind.NewKeyedTransactorWithChainID(d.key, d.chainID)
	if err != nil {
		return nil, err
	}
	opts.Context = ctx
	return opts, nil
}

// awaitMined waits for tx and errors on revert. Mined transactions are
// logged with their hash; manifests record only addresses.
func (d *Driver) awaitMined(ctx context.Context, label string, tx *types.Transaction) error {
	receipt, err := bind.WaitMined(ctx, d.backend, tx)
	if err != nil {
		return fmt.Errorf("%s: wait mined: %w", label, err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("%s: transaction %s reverted", label, tx.Hash())
	}
	slog.Info("transaction mined",
		"label", label,
		"tx", tx.Hash().Hex(),
		"block", receipt.BlockNumber,
		"chain", d.chainID,
	)
	return nil
}
