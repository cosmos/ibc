// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ift"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/link/internal/deploy"
	"github.com/cosmos/ibc/link/internal/deploy/manifest"
)

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

// Custom-error selectors for the getters that revert when absent, so their
// reverts can be classified as not-found.
var (
	clientNotFoundSelector    = customErrorSelector(ics26router.ContractMetaData, "IBCClientNotFound")
	appNotFoundSelector       = customErrorSelector(ics26router.ContractMetaData, "IBCAppNotFound")
	iftBridgeNotFoundSelector = customErrorSelector(ift.ContractMetaData, "IFTBridgeNotFound")
)

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
	if err == nil {
		return address.Hex(), true, nil
	}
	switch classifyGetClientError(err) {
	case clientNotFound:
		return "", false, nil
	case unstructuredRevert:
		// IBCClientNotFound is getClient's only revert on a healthy router,
		// but a provider that strips revert data hides the selector. Probe
		// the router before concluding the client is absent, so a lookup
		// against a broken router stops here instead of spending gas on a
		// client deployment whose registration then fails.
		if _, probeErr := contract.GetNextClientSeq(&bind.CallOpts{Context: ctx}); probeErr != nil {
			return "", false, fmt.Errorf(
				"router %s failed a health probe after an unstructured revert from getClient (%w): %w",
				router, err, probeErr,
			)
		}
		return "", false, nil
	default:
		return "", false, err
	}
}

// getClientError classifies a getClient failure.
type getClientError int

const (
	// otherError is a transport failure or a structured revert with a
	// different selector; callers propagate it.
	otherError getClientError = iota
	// clientNotFound is a definitive IBCClientNotFound revert.
	clientNotFound
	// unstructuredRevert is a revert whose data the provider stripped, so
	// IBCClientNotFound cannot be distinguished from any other revert.
	unstructuredRevert
)

// classifyGetClientError inspects a getClient error: structured revert data
// is matched against the IBCClientNotFound selector; reverts without data
// are ambiguous and reported as such for the caller to disambiguate.
func classifyGetClientError(err error) getClientError {
	if err == nil {
		return otherError
	}
	var dataErr interface{ ErrorData() any }
	if errors.As(err, &dataErr) {
		if data, ok := dataErr.ErrorData().(string); ok && data != "" && clientNotFoundSelector != "" {
			if strings.HasPrefix(strings.ToLower(data), clientNotFoundSelector) {
				return clientNotFound
			}
			return otherError
		}
	}
	if strings.Contains(err.Error(), "IBCClientNotFound") {
		return clientNotFound
	}
	if strings.Contains(err.Error(), "execution reverted") {
		return unstructuredRevert
	}
	return otherError
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

	if m.GMP != nil && m.GMP.Address != "" {
		gmpCode, codeErr := d.HasCode(ctx, m.GMP.Address)
		if codeErr != nil {
			return report, codeErr
		}
		check("gmp app code present", gmpCode, "no code at "+m.GMP.Address)
		if gmpCode {
			appAddr, registered, appErr := d.AppRegistered(ctx, m.Core.Router, m.GMP.Port)
			if appErr != nil {
				return report, appErr
			}
			check(
				"gmp app registered at port "+m.GMP.Port,
				registered && strings.EqualFold(appAddr, m.GMP.Address),
				fmt.Sprintf("router has %q at port %s, manifest has %s", appAddr, m.GMP.Port, m.GMP.Address),
			)
		}
	}

	for _, tok := range m.Tokens {
		tokenCode, codeErr := d.HasCode(ctx, tok.Address)
		if codeErr != nil {
			return report, codeErr
		}
		check("ift token "+tok.Symbol+" code present", tokenCode, "no code at "+tok.Address)
		if !tokenCode {
			continue
		}
		for _, b := range tok.Bridges {
			cp, _, registered, bridgeErr := d.IFTBridge(ctx, tok.Address, b.ClientID)
			if bridgeErr != nil {
				return report, bridgeErr
			}
			check(
				"ift "+tok.Symbol+" bridge "+b.ClientID+" registered",
				registered && cp == b.CounterpartyIFT,
				fmt.Sprintf("token has %q for client %s, manifest has %s", cp, b.ClientID, b.CounterpartyIFT),
			)
		}
	}
	return report, nil
}

// customErrorSelector returns the 0x-prefixed 4-byte selector of the named
// custom error in md's ABI, or "" if unavailable.
func customErrorSelector(md *bind.MetaData, name string) string {
	parsed, err := md.GetAbi()
	if err != nil || parsed == nil {
		return ""
	}
	e, ok := parsed.Errors[name]
	if !ok {
		return ""
	}
	return "0x" + common.Bytes2Hex(e.ID[:4])
}

// isNotFoundRevert reports whether err is a contract revert for the custom
// error identified by selectorHex/name (getIBCApp and getIFTBridge revert when
// absent rather than returning zero). A data-less revert is treated as a match.
func isNotFoundRevert(err error, selectorHex, name string) bool {
	if err == nil {
		return false
	}
	var dataErr interface{ ErrorData() any }
	if errors.As(err, &dataErr) {
		if data, ok := dataErr.ErrorData().(string); ok && data != "" {
			return selectorHex != "" && strings.HasPrefix(strings.ToLower(data), selectorHex)
		}
	}
	if strings.Contains(err.Error(), name) {
		return true
	}
	return strings.Contains(err.Error(), "execution reverted")
}

// RegisterApp registers app on the router under port via the restricted
// addIBCApp(portId, app) overload.
func (d *Driver) RegisterApp(ctx context.Context, router, app, port string) error {
	contract, err := ics26router.NewContract(common.HexToAddress(router), d.backend)
	if err != nil {
		return err
	}
	opts, err := d.transactOpts(ctx)
	if err != nil {
		return err
	}
	tx, err := contract.AddIBCApp0(opts, port, common.HexToAddress(app))
	if err != nil {
		return fmt.Errorf("addIBCApp %q: %w", port, err)
	}
	return d.awaitMined(ctx, "addIBCApp "+port, tx)
}

// AppRegistered queries the router for the app at port.
func (d *Driver) AppRegistered(ctx context.Context, router, port string) (string, bool, error) {
	contract, err := ics26router.NewContract(common.HexToAddress(router), d.backend)
	if err != nil {
		return "", false, err
	}
	addr, err := contract.GetIBCApp(&bind.CallOpts{Context: ctx}, port)
	if err == nil {
		return addr.Hex(), true, nil
	}
	if isNotFoundRevert(err, appNotFoundSelector, "IBCAppNotFound") {
		return "", false, nil
	}
	return "", false, err
}

// RegisterIFTBridge registers a bridge on an IFT token.
func (d *Driver) RegisterIFTBridge(ctx context.Context, iftAddr string, spec deploy.BridgeSpec) error {
	if !common.IsHexAddress(spec.SendCallConstructor) {
		return fmt.Errorf("invalid send call constructor address %q", spec.SendCallConstructor)
	}
	if spec.CounterpartyIFT == "" {
		return fmt.Errorf("counterparty ift address required")
	}
	contract, err := ift.NewContract(common.HexToAddress(iftAddr), d.backend)
	if err != nil {
		return err
	}
	opts, err := d.transactOpts(ctx)
	if err != nil {
		return err
	}
	tx, err := contract.RegisterIFTBridge(
		opts,
		spec.ClientID,
		spec.CounterpartyIFT,
		common.HexToAddress(spec.SendCallConstructor),
	)
	if err != nil {
		return fmt.Errorf("registerIFTBridge %q: %w", spec.ClientID, err)
	}
	return d.awaitMined(ctx, "registerIFTBridge "+spec.ClientID, tx)
}

// IFTBridge queries the token for a bridge registered under clientID, returning
// its counterparty address and send-call constructor.
func (d *Driver) IFTBridge(ctx context.Context, iftAddr, clientID string) (string, string, bool, error) {
	contract, err := ift.NewContract(common.HexToAddress(iftAddr), d.backend)
	if err != nil {
		return "", "", false, err
	}
	bridge, err := contract.GetIFTBridge(&bind.CallOpts{Context: ctx}, clientID)
	if err == nil {
		return bridge.CounterpartyIFTAddress, bridge.IftSendCallConstructor.Hex(), true, nil
	}
	if isNotFoundRevert(err, iftBridgeNotFoundSelector, "IFTBridgeNotFound") {
		return "", "", false, nil
	}
	return "", "", false, err
}
