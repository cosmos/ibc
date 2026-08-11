// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
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
	return report, nil
}
