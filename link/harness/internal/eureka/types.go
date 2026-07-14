// Package eureka realizes the concrete Solidity IBC Eureka EVM protocol state.
package eureka

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Instance is one initialized ICS26 router installation. The router address is
// the ERC1967 proxy used by clients and relayers; AccessManager controls its
// restricted operations.
type Instance struct {
	AccessManager common.Address
	Router        common.Address
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
// submit proofs, matching Eureka's constructor semantics.
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
		return fmt.Errorf("client id %q is not a valid Eureka custom client identifier", c.ID)
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

type TransactionSubmission string

const (
	TransactionSubmissionAccepted  TransactionSubmission = "accepted"
	TransactionSubmissionAmbiguous TransactionSubmission = "ambiguous"
)

// TransactionEvidence is the non-secret setup transaction evidence available
// so far. Hash is recorded before broadcast. Submission distinguishes an RPC-
// accepted transaction from one whose broadcast result was ambiguous, while
// Mined reports whether a receipt was observed.
type TransactionEvidence struct {
	Hash                     common.Hash
	Submission               TransactionSubmission
	Mined                    bool
	BlockNumber              uint64
	Status                   uint64
	PredictedContractAddress common.Address
	ContractAddress          common.Address
}

func minedTransactionEvidence(receipt *types.Receipt) TransactionEvidence {
	var blockNumber uint64
	if receipt.BlockNumber != nil {
		blockNumber = receipt.BlockNumber.Uint64()
	}
	return TransactionEvidence{
		Hash:            receipt.TxHash,
		Submission:      TransactionSubmissionAccepted,
		Mined:           true,
		BlockNumber:     blockNumber,
		Status:          receipt.Status,
		ContractAddress: receipt.ContractAddress,
	}
}

func signedTransactionEvidence(tx *types.Transaction, status TransactionSubmission) *TransactionEvidence {
	if tx == nil {
		return nil
	}
	return &TransactionEvidence{Hash: tx.Hash(), Submission: status}
}

// InstanceReceipts is populated after each transaction is signed. Callers get
// the known prefix even when broadcast is ambiguous or a later stage fails.
type InstanceReceipts struct {
	AccessManager        *TransactionEvidence
	RouterImplementation *TransactionEvidence
	RouterProxy          *TransactionEvidence
}

// ClientReceipts preserves light-client and registration transaction evidence
// independently when broadcast, mining, or a later stage fails.
type ClientReceipts struct {
	LightClient  *TransactionEvidence
	Registration *TransactionEvidence
}

// validCustomClientID mirrors Eureka v3's IBCIdentifiers validation. The
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
