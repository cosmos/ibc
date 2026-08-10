// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
)

const (
	recordPrefix   = e2etest.MatrixRecordPrefix
	modeFast       = string(e2etest.ModeFast)
	modeComplete   = string(e2etest.ModeComplete)
	modeProduction = string(e2etest.ModeProduction)
	actionPass     = "pass"
)

var modes = []string{modeFast, modeComplete, modeProduction}

type options struct {
	path  string
	check bool
}

type record = e2etest.MatrixRecord

type discovery struct {
	records []record
	passed  map[string]bool
}

type testEvent struct {
	Action string
	Test   string
	Output string
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "e2e-matrix:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	tests, err := listTests(ctx)
	if err != nil {
		return err
	}
	discoveries := make(map[string]discovery, len(modes))
	for _, mode := range modes {
		discoveries[mode], err = discover(ctx, mode)
		if err != nil {
			return err
		}
	}
	generated, err := render(tests, discoveries)
	if err != nil {
		return err
	}
	if !opts.check {
		return os.WriteFile(opts.path, generated, 0o644)
	}
	want, err := os.ReadFile(opts.path)
	if err != nil {
		return err
	}
	if !bytes.Equal(want, generated) {
		return fmt.Errorf("%s is out of date; run go run ./cmd/e2e-matrix -write %s", opts.path, opts.path)
	}
	return nil
}

func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("e2e-matrix", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	write := flags.String("write", "", "write the generated matrix")
	check := flags.String("check", "", "check the generated matrix")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 || (*write == "") == (*check == "") {
		return options{}, errors.New("provide exactly one of -write <path> or -check <path>")
	}
	if *write != "" {
		return options{path: *write}, nil
	}
	return options{path: *check, check: true}, nil
}

func listTests(ctx context.Context) ([]string, error) {
	output, err := exec.CommandContext(ctx, "go", "test", "-list", "^Test", ".").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("enumerate tests: %w\n%s", err, output)
	}
	var tests []string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "Test") && !strings.ContainsAny(line, " \t") {
			tests = append(tests, line)
		}
	}
	sort.Strings(tests)
	if len(tests) == 0 {
		return nil, errors.New("enumerate tests: no top-level tests found")
	}
	return tests, nil
}

func discover(ctx context.Context, mode string) (discovery, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "-json", "-count=1", ".", "-args", "-e2e.matrix")
	cmd.Env = modeEnvironment(mode)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return discovery{}, fmt.Errorf(
			"discover %s mode: %w\nstdout:\n%s\nstderr:\n%s",
			mode,
			err,
			tail(stdout.Bytes(), 8000),
			tail(stderr.Bytes(), 8000),
		)
	}
	parsed, parseErr := parseDiscovery(&stdout)
	if parseErr != nil {
		return discovery{}, fmt.Errorf("discover %s mode: %w", mode, parseErr)
	}
	return parsed, nil
}

func modeEnvironment(mode string) []string {
	return append(os.Environ(), "E2E_MODE="+mode)
}

func parseDiscovery(input io.Reader) (discovery, error) {
	result := discovery{passed: make(map[string]bool)}
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var event testEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return discovery{}, fmt.Errorf("parse go test JSON: %w", err)
		}
		if event.Action == actionPass && event.Test != "" && !strings.Contains(event.Test, "/") {
			result.passed[event.Test] = true
		}
		line := strings.TrimSpace(event.Output)
		encoded, ok := strings.CutPrefix(line, recordPrefix)
		if !ok {
			continue
		}
		var entry record
		if err := json.Unmarshal([]byte(encoded), &entry); err != nil {
			return discovery{}, fmt.Errorf("parse matrix record: %w", err)
		}
		if entry.Mode == "" || entry.Test == "" || (entry.Spec == nil) == (entry.Outcome == "") {
			return discovery{}, errors.New("parse matrix record: missing mode, test, or terminal result")
		}
		key := string(entry.Mode) + "\x00" + entry.Test
		if seen[key] {
			return discovery{}, fmt.Errorf("duplicate matrix record for %s in %s mode", entry.Test, entry.Mode)
		}
		seen[key] = true
		result.records = append(result.records, entry)
	}
	if err := scanner.Err(); err != nil {
		return discovery{}, err
	}
	return result, nil
}

func render(tests []string, discoveries map[string]discovery) ([]byte, error) {
	tests = append([]string(nil), tests...)
	sort.Strings(tests)
	known := make(map[string]bool, len(tests))
	for _, test := range tests {
		known[test] = true
	}
	grouped := make(map[string]map[string][]record, len(tests))
	for _, mode := range modes {
		grouped[mode] = make(map[string][]record)
		for _, record := range discoveries[mode].records {
			if record.Mode != e2etest.Mode(mode) {
				return nil, fmt.Errorf(
					"record for %s reports mode %s during %s discovery",
					record.Test,
					record.Mode,
					mode,
				)
			}
			top := topLevel(record.Test)
			if !known[top] {
				return nil, fmt.Errorf("matrix record references unlisted test %s", record.Test)
			}
			grouped[mode][top] = append(grouped[mode][top], record)
		}
		for _, test := range tests {
			if len(grouped[mode][test]) == 0 && !discoveries[mode].passed[test] {
				return nil, fmt.Errorf("%s produced neither a matrix record nor a pass in %s mode", test, mode)
			}
		}
	}

	var out strings.Builder
	out.WriteString("<!-- SPDX-License-Identifier: Apache-2.0 -->\n\n")
	out.WriteString("# E2E Test Matrix\n\n")
	out.WriteString("<!-- Code generated by go run ./cmd/e2e-matrix; DO NOT EDIT. -->\n\n")
	out.WriteString("| Test | Requirements | Fast | Complete | Production |\n")
	out.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, test := range tests {
		fmt.Fprintf(&out, "| `%s` | %s", escape(test), formatRequirements(test, grouped))
		for _, mode := range modes {
			fmt.Fprintf(&out, " | %s", formatMode(test, grouped[mode][test]))
		}
		out.WriteString(" |\n")
	}
	return []byte(out.String()), nil
}

func formatRequirements(test string, grouped map[string]map[string][]record) string {
	set := make(map[string]bool)
	for _, mode := range modes {
		for _, record := range grouped[mode][test] {
			for _, selection := range record.Selections {
				set[formatRequirement(selection)] = true
			}
		}
	}
	if len(set) == 0 {
		return "None"
	}
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return strings.Join(values, "; ")
}

func formatRequirement(selection e2etest.MatrixSelection) string {
	parts := make([]string, 0, 3)
	if selection.Requirements.Provider != "" {
		parts = append(parts, "provider "+string(selection.Requirements.Provider))
	}
	if selection.Requirements.ControlledMining {
		parts = append(parts, "controlled mining")
	}
	if selection.Requirements.NodeLifecycle {
		parts = append(parts, "node lifecycle")
	}
	if len(parts) == 0 {
		return selection.Family + " portable"
	}
	return selection.Family + " (" + strings.Join(parts, ", ") + ")"
}

func formatMode(top string, records []record) string {
	if len(records) == 0 {
		return "No environment"
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Test < records[j].Test })
	values := make([]string, 0, len(records))
	for _, record := range records {
		value := ""
		if record.Outcome != "" {
			value = strings.ToUpper(record.Outcome[:1]) + record.Outcome[1:] + ": " + record.Reason
		} else {
			value = formatSpec(*record.Spec)
		}
		if record.Test != top {
			value = strings.TrimPrefix(record.Test, top+"/") + ": " + value
		}
		values = append(values, escape(value))
	}
	return strings.Join(values, "<br>")
}

func formatSpec(summary e2etest.MatrixSpec) string {
	parts := make([]string, 0, len(summary.Chains)+3)
	for _, chain := range summary.Chains {
		parts = append(parts, strconv.Itoa(chain.Count)+"× "+chain.Type)
	}
	parts = append(parts,
		count(summary.IBCInstances, "IBC instance"),
		count(summary.Connections, "connection"),
		count(summary.Attestors, "attestor"),
	)
	return strings.Join(parts, "; ")
}

func count(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return strconv.Itoa(n) + " " + noun
}

func topLevel(name string) string {
	if before, _, ok := strings.Cut(name, "/"); ok {
		return before
	}
	return name
}

func escape(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func tail(value []byte, limit int) []byte {
	if len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}
