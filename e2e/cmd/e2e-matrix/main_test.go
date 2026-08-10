// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
)

func TestParseOptionsRequiresExactlyOneAction(t *testing.T) {
	for _, args := range [][]string{nil, {"-write", "a", "-check", "b"}, {"-write", ""}, {"extra"}} {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("parseOptions(%q) succeeded", args)
		}
	}
	write, err := parseOptions([]string{"-write", "matrix.md"})
	if err != nil || write.path != "matrix.md" || write.check {
		t.Fatalf("write options = %#v, %v", write, err)
	}
	check, err := parseOptions([]string{"-check", "matrix.md"})
	if err != nil || check.path != "matrix.md" || !check.check {
		t.Fatalf("check options = %#v, %v", check, err)
	}
}

func TestParseDiscoveryRejectsDuplicateRecords(t *testing.T) {
	entry := e2etest.MatrixRecord{Mode: e2etest.ModeFast, Test: "TestA", Spec: &e2etest.MatrixSpec{}}
	line := eventLine(t, testEvent{Action: "output", Output: recordPrefix + mustJSON(t, entry) + "\n"})
	if _, err := parseDiscovery(strings.NewReader(line + line)); err == nil {
		t.Fatal("duplicate record succeeded")
	}
}

func TestRenderGroupsSubtestsNoEnvironmentAndSorts(t *testing.T) {
	tests := []string{"TestZ", "TestA"}
	discoveries := make(map[string]discovery)
	for _, mode := range modes {
		rec := record{
			Mode: e2etest.Mode(mode),
			Test: "TestZ/environment",
			Selections: []e2etest.MatrixSelection{
				{Family: "EVM", Requirements: e2etest.EVMRequirements{}},
				{Family: "EVM", Requirements: e2etest.EVMRequirements{NodeLifecycle: true}},
			},
			Spec: &e2etest.MatrixSpec{
				Chains:       []e2etest.MatrixChain{{Type: "Anvil", Count: 1}, {Type: "Attached EVM", Count: 1}},
				IBCInstances: 2, Connections: 1,
			},
		}
		if mode == modeProduction {
			rec.Spec = nil
			rec.Outcome = "skip"
			rec.Reason = "not available | yet"
		}
		discoveries[mode] = discovery{records: []record{rec}, passed: map[string]bool{"TestA": true, "TestZ": true}}
	}

	first, err := render(tests, discoveries)
	if err != nil {
		t.Fatal(err)
	}
	second, err := render(tests, discoveries)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("render not stable: %v", err)
	}
	text := string(first)
	if strings.Index(text, "`TestA`") > strings.Index(text, "`TestZ`") {
		t.Fatalf("tests not sorted:\n%s", text)
	}
	for _, want := range []string{
		"<!-- SPDX-License-Identifier: Apache-2.0 -->",
		"| `TestA` | None | No environment | No environment | No environment |",
		"EVM (node lifecycle); EVM portable",
		"environment: 1× Anvil; 1× Attached EVM; 2 IBC instances; 1 connection; 0 attestors",
		"Skip: not available \\| yet",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("matrix lacks %q:\n%s", want, text)
		}
	}
}

func TestParseDiscoveryTracksTopLevelPassAndRecord(t *testing.T) {
	entry := e2etest.MatrixRecord{Mode: e2etest.ModeFast, Test: "TestA/sub", Spec: &e2etest.MatrixSpec{}}
	input := eventLine(t, testEvent{Action: "output", Output: recordPrefix + mustJSON(t, entry) + "\n"}) +
		eventLine(t, testEvent{Action: actionPass, Test: "TestA/sub"}) +
		eventLine(t, testEvent{Action: actionPass, Test: "TestA"})
	got, err := parseDiscovery(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.records) != 1 || !got.passed["TestA"] || got.passed["TestA/sub"] {
		t.Fatalf("discovery = %#v", got)
	}
}

func TestDiscoverKeepsStderrOutOfJSON(t *testing.T) {
	dir := t.TempDir()
	goCommand := filepath.Join(dir, "go")
	script := "#!/bin/sh\nprintf 'not json\\n'\nprintf 'go diagnostic\\n' >&2\nexit 1\n"
	if err := os.WriteFile(goCommand, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	_, err := discover(t.Context(), modeFast)
	if err == nil {
		t.Fatal("discover() succeeded")
	}
	if !strings.Contains(err.Error(), "go diagnostic") ||
		strings.Contains(err.Error(), "parse go test JSON") {
		t.Fatalf("discover() error = %v", err)
	}
}

func eventLine(t *testing.T, event testEvent) string {
	t.Helper()
	return mustJSON(t, event) + "\n"
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
