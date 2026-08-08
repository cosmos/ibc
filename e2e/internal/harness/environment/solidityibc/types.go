// Package solidityibc realizes the concrete Solidity IBC EVM protocol state.
package solidityibc

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// Instance is one initialized ICS26 router installation. The router address is
// the ERC1967 proxy used by clients and relayers; AccessManager controls its
// restricted operations.
type Instance struct {
	AccessManager common.Address
	Router        common.Address
}

// AppStack is the deployed IBC application layer bound to one Instance.
type AppStack struct {
	ICS20Transfer common.Address // ERC1967 proxy
	ICS27GMP      common.Address // ERC1967 proxy
}

// Client is an attestation light client registered with one Instance.
type Client struct {
	ID                    string
	Address               common.Address
	CounterpartyClientID  string
	Attestors             []common.Address
	MinRequiredSignatures uint8
}

// AttestationClientConfig contains the immutable constructor inputs for an
// attestation light client. A zero RoleManager intentionally permits anyone to
// submit proofs, matching Solidity IBC's constructor semantics.
type AttestationClientConfig struct {
	ID                    string
	CounterpartyClientID  string
	Attestors             []common.Address
	MinRequiredSignatures uint8
	InitialHeight         uint64
	InitialTimestamp      uint64
	RoleManager           common.Address
}

func (c AttestationClientConfig) snapshot() AttestationClientConfig {
	c.Attestors = slices.Clone(c.Attestors)
	return c
}

func (c AttestationClientConfig) validate() error {
	if !validCustomClientID(c.ID) {
		return fmt.Errorf("client id %q is not a valid Solidity IBC custom client identifier", c.ID)
	}
	if c.CounterpartyClientID == "" {
		return fmt.Errorf("client %q has an empty counterparty client id", c.ID)
	}
	if len(c.Attestors) == 0 {
		return fmt.Errorf("client %q has no attestors", c.ID)
	}
	seen := make(map[common.Address]struct{}, len(c.Attestors))
	for _, address := range c.Attestors {
		if address == (common.Address{}) {
			return fmt.Errorf("client %q has a zero attestor address", c.ID)
		}
		if _, duplicate := seen[address]; duplicate {
			return fmt.Errorf("client %q repeats attestor %s", c.ID, address)
		}
		seen[address] = struct{}{}
	}
	if c.MinRequiredSignatures == 0 || int(c.MinRequiredSignatures) > len(c.Attestors) {
		return fmt.Errorf(
			"client %q requires %d signatures from %d attestors",
			c.ID,
			c.MinRequiredSignatures,
			len(c.Attestors),
		)
	}
	if c.InitialHeight == 0 {
		return fmt.Errorf("client %q has zero initial height", c.ID)
	}
	if c.InitialTimestamp == 0 {
		return fmt.Errorf("client %q has zero initial timestamp", c.ID)
	}
	return nil
}

// validCustomClientID mirrors Solidity IBC v3's IBCIdentifiers validation. The
// access-controlled addClient overload rejects generated "client-" identifiers
// and accepts only these explicit custom identifiers.
func validCustomClientID(id string) bool {
	if len(id) < 4 || len(id) > 128 || strings.HasPrefix(id, "channel-") || strings.HasPrefix(id, "client-") {
		return false
	}
	for _, b := range []byte(id) {
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' {
			continue
		}
		switch b {
		case '.', '_', '+', '-', '#', '[', ']', '<', '>':
			continue
		default:
			return false
		}
	}
	return true
}
