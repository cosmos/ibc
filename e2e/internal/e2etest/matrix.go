package e2etest

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"testing"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

const (
	MatrixRecordPrefix = "E2E_MATRIX_RECORD="
	matrixFamilyEVM    = "EVM"
)

var (
	matrixFlag = flag.Bool("e2e.matrix", false, "discover the e2e provider/topology matrix")
	matrix     = newMatrixCollector(os.Stdout)
)

type matrixKey struct {
	mode Mode
	test string
}

// MatrixSelection is one provider selection captured during matrix discovery.
type MatrixSelection struct {
	Family       string          `json:"family"`
	Requirements EVMRequirements `json:"requirements"`
	Provider     EVMProvider     `json:"provider,omitempty"`
}

// MatrixChain summarizes a group of resolved Chains of the same type.
type MatrixChain struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// MatrixSpec summarizes a resolved environment Spec.
type MatrixSpec struct {
	Chains       []MatrixChain `json:"chains"`
	IBCInstances int           `json:"ibc_instances"`
	Connections  int           `json:"connections"`
	Attestors    int           `json:"attestors"`
}

// MatrixRecord is the JSON record exchanged with the matrix generator.
type MatrixRecord struct {
	Mode       Mode              `json:"mode"`
	Test       string            `json:"test"`
	Selections []MatrixSelection `json:"selections,omitempty"`
	Spec       *MatrixSpec       `json:"spec,omitempty"`
	Outcome    string            `json:"outcome,omitempty"`
	Reason     string            `json:"reason,omitempty"`
}

type matrixCollector struct {
	mu      sync.Mutex
	out     io.Writer
	records map[matrixKey]*MatrixRecord
}

func newMatrixCollector(out io.Writer) *matrixCollector {
	return &matrixCollector{out: out, records: make(map[matrixKey]*MatrixRecord)}
}

func matrixDiscoveryEnabled() bool { return *matrixFlag }

func recordEVMSelection(
	t testing.TB,
	mode Mode,
	requirements EVMRequirements,
	provider EVMProvider,
	outcome string,
	reason string,
) {
	t.Helper()
	if !matrixDiscoveryEnabled() {
		return
	}
	recordMatrix(t, matrix.recordSelection(t.Name(), mode, MatrixSelection{
		Family: matrixFamilyEVM, Requirements: requirements, Provider: provider,
	}, outcome, reason))
}

func recordResolvedSpec(t testing.TB, mode Mode, spec environment.Spec) {
	t.Helper()
	if !matrixDiscoveryEnabled() {
		return
	}
	recordMatrix(t, matrix.recordSpec(t.Name(), mode, summarizeSpec(spec)))
}

func recordMatrix(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("e2etest: %v", err)
	}
}

func (c *matrixCollector) recordSelection(
	test string,
	mode Mode,
	selection MatrixSelection,
	outcome string,
	reason string,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	record := c.pending(test, mode)
	record.Selections = append(record.Selections, selection)
	if outcome == "" {
		return nil
	}
	record.Outcome = outcome
	record.Reason = reason
	return c.emit(record)
}

func (c *matrixCollector) recordSpec(test string, mode Mode, spec MatrixSpec) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	record := c.pending(test, mode)
	record.Spec = &spec
	return c.emit(record)
}

func (c *matrixCollector) pending(test string, mode Mode) *MatrixRecord {
	key := matrixKey{mode: mode, test: test}
	record := c.records[key]
	if record == nil {
		record = &MatrixRecord{Mode: mode, Test: test}
		c.records[key] = record
	}
	return record
}

func (c *matrixCollector) emit(record *MatrixRecord) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal matrix record: %w", err)
	}
	if _, err := fmt.Fprintf(c.out, "%s%s\n", MatrixRecordPrefix, encoded); err != nil {
		return fmt.Errorf("write matrix record: %w", err)
	}
	return nil
}

func summarizeSpec(spec environment.Spec) MatrixSpec {
	counts := make(map[string]int)
	for _, chain := range spec.Chains {
		switch chain.(type) {
		case environment.ManagedAnvil:
			counts["Anvil"]++
		case environment.ManagedBesu:
			counts["Besu"]++
		case environment.AttachedEVM:
			counts["Attached EVM"]++
		default:
			counts[fmt.Sprintf("%T", chain)]++
		}
	}
	types := make([]string, 0, len(counts))
	for typ := range counts {
		types = append(types, typ)
	}
	sort.Strings(types)
	chains := make([]MatrixChain, 0, len(types))
	for _, typ := range types {
		chains = append(chains, MatrixChain{Type: typ, Count: counts[typ]})
	}
	return MatrixSpec{
		Chains:       chains,
		IBCInstances: len(spec.IBCInstances),
		Connections:  len(spec.Connections),
		Attestors:    len(spec.Attestors),
	}
}
