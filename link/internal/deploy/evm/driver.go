// Package evm implements the deploy Target for EVM chains. All contract
// creation bytecode ships inside the binary (go-abigen artifacts plus the
// embedded AccessManager artifact), so deployment needs only an RPC endpoint.
package evm

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/attestation"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/erc1967proxy"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/link/internal/deploy"
)

// contractsVersion identifies the pinned contract artifacts. Keep in sync
// with the go-abigen version in go.mod.
const contractsVersion = "go-abigen v0.0.0-20260618122836-39904319467b"

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

func (d *Driver) ContractsVersion() string { return contractsVersion }

// DeployerAddress returns the deployer key's address for provenance, or ""
// for a read-only driver.
func (d *Driver) DeployerAddress() string {
	if d.key == nil {
		return ""
	}
	return crypto.PubkeyToAddress(d.key.PublicKey).Hex()
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
	if err := d.awaitMined(ctx, "deploy AccessManager", amTx); err != nil {
		return deploy.CoreRef{}, err
	}

	implAddr, implTx, _, err := ics26router.DeployContract(opts, d.backend)
	if err != nil {
		return deploy.CoreRef{}, fmt.Errorf("deploy ICS26Router implementation: %w", err)
	}
	if err := d.awaitMined(ctx, "deploy ICS26Router implementation", implTx); err != nil {
		return deploy.CoreRef{}, err
	}

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	if err != nil {
		return deploy.CoreRef{}, err
	}
	init, err := routerABI.Pack("initialize", amAddr)
	if err != nil {
		return deploy.CoreRef{}, err
	}
	routerAddr, routerTx, _, err := erc1967proxy.DeployContract(opts, d.backend, implAddr, init)
	if err != nil {
		return deploy.CoreRef{}, fmt.Errorf("deploy ICS26Router proxy: %w", err)
	}
	if err := d.awaitMined(ctx, "deploy ICS26Router proxy", routerTx); err != nil {
		return deploy.CoreRef{}, err
	}

	// OZ AccessManager PUBLIC_ROLE is type(uint64).max
	roleTx, err := am.Transact(opts, "setTargetFunctionRole", routerAddr, selectors, uint64(math.MaxUint64))
	if err != nil {
		return deploy.CoreRef{}, fmt.Errorf("setTargetFunctionRole: %w", err)
	}
	if err := d.awaitMined(ctx, "setTargetFunctionRole", roleTx); err != nil {
		return deploy.CoreRef{}, err
	}

	return deploy.CoreRef{
		Router: routerAddr.Hex(),
		TargetData: map[string]string{
			"accessManager":             amAddr.Hex(),
			"ics26RouterImplementation": implAddr.Hex(),
		},
		TxHashes: map[string]string{
			"core-0": amTx.Hash().Hex(),
			"core-1": implTx.Hash().Hex(),
			"core-2": routerTx.Hash().Hex(),
			"core-3": roleTx.Hash().Hex(),
		},
	}, nil
}

// ProvisionClient deploys a light client contract.
func (d *Driver) ProvisionClient(ctx context.Context, spec deploy.ClientSpec) (deploy.ClientRef, error) {
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
		opts, d.backend, attestors, params.Threshold, params.InitialHeight, params.InitialTimestamp, common.Address{},
	)
	if err != nil {
		return deploy.ClientRef{}, fmt.Errorf("deploy attestation client: %w", err)
	}
	if err := d.awaitMined(ctx, "deploy attestation client", tx); err != nil {
		return deploy.ClientRef{}, err
	}
	return deploy.ClientRef{Address: addr.Hex(), TxHash: tx.Hash().Hex()}, nil
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

// awaitMined waits for tx and errors on revert.
func (d *Driver) awaitMined(ctx context.Context, label string, tx *types.Transaction) error {
	receipt, err := bind.WaitMined(ctx, d.backend, tx)
	if err != nil {
		return fmt.Errorf("%s: wait mined: %w", label, err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("%s: transaction %s reverted", label, tx.Hash())
	}
	return nil
}
