// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

func TestMatrixCollectorJoinsSelectionsAndSpec(t *testing.T) {
	var output bytes.Buffer
	collector := newMatrixCollector(&output)
	if err := collector.recordSelection("TestMesh/subtest", ModeProduction, MatrixSelection{
		Family: matrixFamilyEVM, Requirements: EVMRequirements{}, Provider: EVMProviderBesu,
	}, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := collector.recordSelection("TestMesh/subtest", ModeProduction, MatrixSelection{
		Family: matrixFamilyEVM, Requirements: EVMRequirements{NodeLifecycle: true}, Provider: EVMProviderAnvil,
	}, "", ""); err != nil {
		t.Fatal(err)
	}
	summary := summarizeSpec(environment.Spec{
		Chains: []environment.ChainSpec{
			environment.ManagedBesu{ID: "a", EVMChainID: 1},
			environment.AttachedEVM{ID: "b", EVMChainID: 2},
		},
		IBCInstances: make([]environment.IBCInstanceSpec, 2),
		Connections:  make([]environment.ConnectionSpec, 1),
		Attestors:    make([]environment.AttestorSpec, 3),
	})
	if err := collector.recordSpec("TestMesh/subtest", ModeProduction, summary); err != nil {
		t.Fatal(err)
	}
	record := decodeMatrixRecord(t, strings.TrimSpace(output.String()))
	if len(record.Selections) != 2 || record.Spec == nil || record.Spec.Attestors != 3 {
		t.Fatalf("joined record = %#v", record)
	}
	if got := record.Spec.Chains; len(got) != 2 || got[0].Type != "Attached EVM" || got[1].Type != "Besu" {
		t.Fatalf("sorted chain summary = %#v", got)
	}
}

func TestMatrixCollectorEmitsSkip(t *testing.T) {
	var output bytes.Buffer
	collector := newMatrixCollector(&output)
	if err := collector.recordSelection("TestSkipped", ModeFast, MatrixSelection{
		Family: matrixFamilyEVM, Requirements: EVMRequirements{Provider: EVMProviderBesu},
	}, "skip", "no compatible provider"); err != nil {
		t.Fatal(err)
	}
	record := decodeMatrixRecord(t, strings.TrimSpace(output.String()))
	if record.Outcome != "skip" || record.Reason != "no compatible provider" || record.Spec != nil {
		t.Fatalf("skip record = %#v", record)
	}
}

func TestMatrixStartValidatesInputs(t *testing.T) {
	if os.Getenv("E2E_MATRIX_VALIDATE_HELPER") == "1" {
		Start(t, environment.Spec{Chains: []environment.ChainSpec{
			environment.ManagedAnvil{EVMChainID: 31337},
		}}, environment.Runtime{})
		return
	}

	output, err := runMatrixHelper("TestMatrixStartValidatesInputs", "E2E_MATRIX_VALIDATE_HELPER=1")
	if err == nil || strings.Contains(output, MatrixRecordPrefix) || !strings.Contains(output, "validate Environment") {
		t.Fatalf("matrix validation output = %q, error = %v", output, err)
	}
}

func TestInvalidMatrixModeDoesNotEmitRecord(t *testing.T) {
	if os.Getenv("E2E_MATRIX_INVALID_MODE_HELPER") == "1" {
		EVMChains(t, EVMRequirements{}, ChainA)
		return
	}

	output, err := runMatrixHelper(
		"TestInvalidMatrixModeDoesNotEmitRecord",
		"E2E_MATRIX_INVALID_MODE_HELPER=1",
		"-e2e.mode=invalid",
	)
	if err == nil || strings.Contains(output, MatrixRecordPrefix) || !strings.Contains(output, "unknown e2e mode") {
		t.Fatalf("invalid mode output = %q, error = %v", output, err)
	}
}

func TestMatrixCollectorParallelWritesAreWhole(t *testing.T) {
	var output bytes.Buffer
	collector := newMatrixCollector(&output)
	var group sync.WaitGroup
	for i := range 32 {
		group.Add(1)
		go func() {
			defer group.Done()
			name := fmt.Sprintf("TestParallel/%02d", i)
			if err := collector.recordSelection(
				name,
				ModeFast,
				MatrixSelection{Family: matrixFamilyEVM},
				"",
				"",
			); err != nil {
				t.Errorf("record selection: %v", err)
				return
			}
			if err := collector.recordSpec(name, ModeFast, MatrixSpec{}); err != nil {
				t.Errorf("record Spec: %v", err)
			}
		}()
	}
	group.Wait()

	var names []string
	scanner := bufio.NewScanner(&output)
	for scanner.Scan() {
		names = append(names, decodeMatrixRecord(t, scanner.Text()).Test)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	if len(names) != 32 || names[0] != "TestParallel/00" || names[31] != "TestParallel/31" {
		t.Fatalf("parallel records = %v", names)
	}
}

func decodeMatrixRecord(t *testing.T, line string) MatrixRecord {
	t.Helper()
	encoded, ok := strings.CutPrefix(line, MatrixRecordPrefix)
	if !ok {
		t.Fatalf("record lacks prefix: %q", line)
	}
	var record MatrixRecord
	if err := json.Unmarshal([]byte(encoded), &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func runMatrixHelper(testName string, environmentValue string, args ...string) (string, error) {
	commandArgs := []string{"-test.run", "^" + testName + "$", "-test.v", "-e2e.matrix"}
	commandArgs = append(commandArgs, args...)
	cmd := exec.Command(os.Args[0], commandArgs...)
	cmd.Env = append(os.Environ(), environmentValue)
	output, err := cmd.CombinedOutput()
	return string(output), err
}
