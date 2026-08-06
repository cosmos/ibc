package evm

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/link/internal/deploy"
	"github.com/cosmos/ibc/link/internal/deploy/manifest"
)

// newTestDriver builds a Driver on an injected backend, without RPC or forge.
// Only exercised from _test.go files, which golangci-lint's unused check
// (run.tests: false) can't see.
//
//nolint:unused
func newTestDriver(chainID *big.Int, b backend, key *ecdsa.PrivateKey) *Driver {
	return &Driver{chainID: chainID, backend: b, key: key}
}

// evmMerklePrefix is the empty prefix EVM counterparties use.
var evmMerklePrefix = [][]byte{{}}

// publicRelayingMethods are opened to PUBLIC_ROLE so any relayer EOA can
// submit packets and client updates.
var publicRelayingMethods = []string{
	"recvPacket",
	"ackPacket",
	"timeoutPacket",
	"updateClient",
	"multicall",
	"submitMisbehaviour",
}

// RegisterClient calls ICS26Router.addClient with the custom client ID.
func (d *Driver) RegisterClient(
	ctx context.Context,
	router string,
	spec deploy.ClientSpec,
	ref deploy.ClientRef,
) (string, error) {
	contract, err := ics26router.NewContract(common.HexToAddress(router), d.backend)
	if err != nil {
		return "", err
	}
	opts, err := d.transactOpts(ctx)
	if err != nil {
		return "", err
	}
	tx, err := contract.AddClient(opts, spec.ClientID, ics26router.IICS02ClientMsgsCounterpartyInfo{
		ClientId:     spec.CounterpartyClientID,
		MerklePrefix: evmMerklePrefix,
	}, common.HexToAddress(ref.Address))
	if err != nil {
		return "", fmt.Errorf("addClient %q: %w", spec.ClientID, err)
	}
	if err := d.awaitMined(ctx, "addClient "+spec.ClientID, tx); err != nil {
		return "", err
	}
	return spec.ClientID, nil
}

// ClientRegistered queries the router for clientID.
func (d *Driver) ClientRegistered(ctx context.Context, router, clientID string) (string, bool, error) {
	contract, err := ics26router.NewContract(common.HexToAddress(router), d.backend)
	if err != nil {
		return "", false, err
	}
	address, err := contract.GetClient(&bind.CallOpts{Context: ctx}, clientID)
	if err != nil {
		if isClientNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return address.Hex(), true, nil
}

// clientNotFoundSelector is the 4-byte selector of the router's
// IBCClientNotFound custom error, hex-encoded with 0x prefix.
var clientNotFoundSelector = func() string {
	routerABI, err := ics26router.ContractMetaData.GetAbi()
	if err != nil || routerABI == nil {
		return ""
	}
	e, ok := routerABI.Errors["IBCClientNotFound"]
	if !ok {
		return ""
	}
	return "0x" + common.Bytes2Hex(e.ID[:4])
}()

// isClientNotFound detects the router's IBCClientNotFound revert. When the
// error carries structured revert data, only that error's selector matches;
// otherwise falls back to substring detection.
func isClientNotFound(err error) bool {
	if err == nil {
		return false
	}
	var dataErr interface{ ErrorData() any }
	if errors.As(err, &dataErr) {
		if data, ok := dataErr.ErrorData().(string); ok && clientNotFoundSelector != "" {
			return strings.HasPrefix(strings.ToLower(data), clientNotFoundSelector)
		}
	}
	return strings.Contains(err.Error(), "IBCClientNotFound") ||
		strings.Contains(err.Error(), "execution reverted")
}

func (d *Driver) HasCode(ctx context.Context, address string) (bool, error) {
	code, err := d.backend.CodeAt(ctx, common.HexToAddress(address), nil)
	if err != nil {
		return false, err
	}
	return len(code) > 0, nil
}

// Head returns the chain head height and timestamp in seconds.
func (d *Driver) Head(ctx context.Context) (uint64, uint64, error) {
	header, err := d.backend.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	return header.Number.Uint64(), header.Time, nil
}

// publicRelayingSelectors resolves the relaying method selectors that
// ProvisionCore binds to AccessManager's PUBLIC_ROLE.
func publicRelayingSelectors() ([][4]byte, error) {
	routerABI, err := ics26router.ContractMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	selectors := make([][4]byte, 0, len(publicRelayingMethods))
	for _, name := range publicRelayingMethods {
		method, ok := routerABI.Methods[name]
		if !ok {
			return nil, fmt.Errorf("router ABI has no method %q", name)
		}
		selectors = append(selectors, [4]byte(method.ID))
	}
	return selectors, nil
}

// Verify checks the manifest against live chain state.
func (d *Driver) Verify(ctx context.Context, m *manifest.Manifest) (deploy.Report, error) {
	var report deploy.Report
	check := func(name string, ok bool, detail string) {
		status := deploy.CheckOK
		if !ok {
			status = deploy.CheckFailed
		} else {
			detail = ""
		}
		report.Checks = append(report.Checks, deploy.Check{Name: name, Status: status, Detail: detail})
	}

	check("manifest chain id matches connected chain", m.ChainID == d.chainID.String(),
		fmt.Sprintf("manifest is for chain %s, connected chain is %s", m.ChainID, d.chainID))
	if m.ChainID != d.chainID.String() {
		return report, nil
	}

	if m.Core.Router == "" {
		check("core router recorded", false, "manifest has no router")
		return report, nil
	}
	hasCode, err := d.HasCode(ctx, m.Core.Router)
	if err != nil {
		return report, err
	}
	check("router code present", hasCode, "no contract code at "+m.Core.Router)
	if !hasCode {
		return report, nil
	}

	contract, err := ics26router.NewContract(common.HexToAddress(m.Core.Router), d.backend)
	if err != nil {
		return report, err
	}
	for _, c := range m.Clients {
		registered, isRegistered, err := d.ClientRegistered(ctx, m.Core.Router, c.ClientID)
		if err != nil {
			return report, err
		}
		check("client "+c.ClientID+" registered", isRegistered, "not registered on router")
		if !isRegistered {
			continue
		}
		check(
			"client "+c.ClientID+" address matches",
			strings.EqualFold(registered, c.Address),
			fmt.Sprintf("router has %s, manifest has %s", registered, c.Address),
		)
		cp, err := contract.GetCounterparty(&bind.CallOpts{Context: ctx}, c.ClientID)
		if err != nil {
			return report, err
		}
		check(
			"client "+c.ClientID+" counterparty matches",
			cp.ClientId == c.CounterpartyClientID,
			fmt.Sprintf("router has %q, manifest has %q", cp.ClientId, c.CounterpartyClientID),
		)
	}
	return report, nil
}
