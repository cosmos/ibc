package evm

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/link/internal/deploy"
)

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
	rpcURL  string
	key     *ecdsa.PrivateKey
	backend backend
	ws      *Workspace
}

// Options configures an EVM driver.
type Options struct {
	ChainID        string
	RPCURL         string
	Home           string
	DeployerKeyHex string
}

// New connects to the chain, validates its ID, and prepares the forge
// workspace. empty DeployerKeyHex builds a read-only driver: queries only,
// no provisioning or wiring.
func New(ctx context.Context, opts Options) (*Driver, error) {
	var key *ecdsa.PrivateKey
	var ws *Workspace
	if opts.DeployerKeyHex != "" {
		if err := EnsureTools(); err != nil {
			return nil, err
		}
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
	if opts.DeployerKeyHex != "" {
		ws, err = EnsureWorkspace(ctx, opts.Home)
		if err != nil {
			return nil, err
		}
	}
	return &Driver{chainID: chainID, rpcURL: opts.RPCURL, key: key, backend: client, ws: ws}, nil
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

func (d *Driver) keyHex() string {
	return common.Bytes2Hex(crypto.FromECDSA(d.key))
}

// checksumAddress converts a hex address (forge's Strings.toHexString
// returns lowercase, no EIP-55 checksum) to go-ethereum's checksummed form,
// so manifest records match what Discover/ClientRegistered produce.
func checksumAddress(addr string) string {
	return common.HexToAddress(addr).Hex()
}

// ProvisionCore deploys AccessManager + ICS26Router proxy via forge; the
// script also opens the relaying selectors to PUBLIC_ROLE.
func (d *Driver) ProvisionCore(ctx context.Context, _ deploy.CoreParams) (deploy.CoreRef, error) {
	if err := d.requireSigner(); err != nil {
		return deploy.CoreRef{}, err
	}
	selectors, err := publicRelayingSelectorsHex()
	if err != nil {
		return deploy.CoreRef{}, err
	}
	returns, txs, err := d.ws.RunScript(ctx, ScriptOptions{
		Script:        "scripts/DeployCore.s.sol:DeployCore",
		ScriptFile:    "DeployCore.s.sol",
		RPCURL:        d.rpcURL,
		ChainID:       d.chainID.String(),
		PrivateKeyHex: d.keyHex(),
		Env:           map[string]string{"IBC_PUBLIC_SELECTORS": selectors},
	})
	if err != nil {
		return deploy.CoreRef{}, err
	}
	router, am := returns["ics26Router"], returns["accessManager"]
	if router == "" || am == "" {
		return deploy.CoreRef{}, fmt.Errorf("forge returned incomplete core addresses: %v", returns)
	}
	txHashes := map[string]string{}
	for i, h := range txs {
		txHashes[fmt.Sprintf("core-%d", i)] = h
	}
	targetData := map[string]string{"accessManager": checksumAddress(am)}
	if impl := returns["ics26RouterImplementation"]; impl != "" {
		targetData["ics26RouterImplementation"] = checksumAddress(impl)
	}
	return deploy.CoreRef{
		Router:     checksumAddress(router),
		TargetData: targetData,
		TxHashes:   txHashes,
	}, nil
}

// ProvisionClient deploys a light client contract via forge.
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
	env, err := attestationEnv(params)
	if err != nil {
		return deploy.ClientRef{}, fmt.Errorf("client %q: %w", spec.ClientID, err)
	}
	returns, txs, err := d.ws.RunScript(ctx, ScriptOptions{
		Script:        "scripts/DeployAttestationClient.s.sol:DeployAttestationClient",
		ScriptFile:    "DeployAttestationClient.s.sol",
		RPCURL:        d.rpcURL,
		ChainID:       d.chainID.String(),
		PrivateKeyHex: d.keyHex(),
		Env:           env,
	})
	if err != nil {
		return deploy.ClientRef{}, err
	}
	address := returns["client"]
	if address == "" {
		return deploy.ClientRef{}, fmt.Errorf("forge returned no client address: %v", returns)
	}
	ref := deploy.ClientRef{Address: checksumAddress(address)}
	if len(txs) > 0 {
		ref.TxHash = txs[len(txs)-1]
	}
	return ref, nil
}

func attestationEnv(p deploy.AttestationParams) (map[string]string, error) {
	if len(p.Attestors) == 0 {
		return nil, fmt.Errorf("attestors required")
	}
	for _, a := range p.Attestors {
		if !common.IsHexAddress(a) {
			return nil, fmt.Errorf("invalid attestor address %q", a)
		}
	}
	if p.Threshold == 0 || int(p.Threshold) > len(p.Attestors) {
		return nil, fmt.Errorf("threshold %d invalid for %d attestors", p.Threshold, len(p.Attestors))
	}
	if p.InitialHeight == 0 || p.InitialTimestamp == 0 {
		return nil, fmt.Errorf("initial height and timestamp required")
	}
	return map[string]string{
		"IBC_ATTESTORS": strings.Join(p.Attestors, ","),
		"IBC_THRESHOLD": strconv.FormatUint(uint64(p.Threshold), 10),
		"IBC_HEIGHT":    strconv.FormatUint(p.InitialHeight, 10),
		"IBC_TIMESTAMP": strconv.FormatUint(p.InitialTimestamp, 10),
	}, nil
}

// transactOpts builds signed transact opts for Go-sent wiring transactions.
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
