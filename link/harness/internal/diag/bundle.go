// Package diag collects failure diagnostics for a run.
package diag

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

type Bundle struct {
	chainLogs  map[string]string
	heights    map[string]uint64
	sutLogs    map[string]string
	statusJSON map[string]string

	versions   map[string]string
	configYAML string
	topology   string
	deployment string
}

func NewBundle() *Bundle {
	return &Bundle{
		chainLogs:  map[string]string{},
		heights:    map[string]uint64{},
		sutLogs:    map[string]string{},
		statusJSON: map[string]string{},
		versions:   map[string]string{},
	}
}

func (b *Bundle) AddChainLog(chainID, log string) {
	b.chainLogs[chainID] = log
}

func (b *Bundle) AddSUTLog(name, log string) {
	b.sutLogs[name] = log
}

func (b *Bundle) AddStatus(name, status string) {
	b.statusJSON[name] = status
}

func (b *Bundle) SetHeight(chainID string, h uint64) {
	b.heights[chainID] = h
}

func (b *Bundle) SetVersion(tool, version string) {
	b.versions[tool] = version
}

func (b *Bundle) SetConfig(yaml string) { b.configYAML = yaml }

func (b *Bundle) SetTopology(summary string) { b.topology = summary }

func (b *Bundle) SetDeployment(d *wire.Deployment) {
	var sb strings.Builder
	for _, id := range sortedKeys(d.Chains) {
		cd := d.Chains[id]
		fmt.Fprintf(&sb, "  chain %s:", id)
		for _, name := range sortedKeys(cd.Fixtures) {
			fmt.Fprintf(&sb, " %s=%s", name, cd.Fixtures[name])
		}
		fmt.Fprintf(&sb, " clientId=%s\n", cd.ClientID)
	}
	if len(d.TxHashes) > 0 {
		fmt.Fprintf(&sb, "  txHashes: %s\n", strings.Join(d.TxHashes, ", "))
	}
	b.deployment = sb.String()
}

func (b *Bundle) String() string {
	var sb strings.Builder
	sb.WriteString("=== diagnostics bundle ===\n")

	if len(b.versions) > 0 {
		sb.WriteString("versions:\n")
		for _, t := range sortedKeys(b.versions) {
			fmt.Fprintf(&sb, "  %s: %s\n", t, b.versions[t])
		}
	}

	if b.topology != "" {
		fmt.Fprintf(&sb, "topology:\n%s\n", indent(b.topology))
	}

	if b.deployment != "" {
		fmt.Fprintf(&sb, "deployment:\n%s", b.deployment)
	}

	sb.WriteString("heights:\n")
	for _, id := range sortedKeys(b.heights) {
		fmt.Fprintf(&sb, "  %s: %d\n", id, b.heights[id])
	}

	if b.configYAML != "" {
		fmt.Fprintf(&sb, "--- resolved config ---\n%s\n", b.configYAML)
	}

	for _, id := range sortedKeys(b.chainLogs) {
		fmt.Fprintf(&sb, "--- chain %s log ---\n%s\n", id, b.chainLogs[id])
	}

	for _, name := range sortedKeys(b.sutLogs) {
		fmt.Fprintf(&sb, "--- sut %s log ---\n%s\n", name, b.sutLogs[name])
	}

	for _, name := range sortedKeys(b.statusJSON) {
		fmt.Fprintf(&sb, "--- sut %s status ---\n%s\n", name, b.statusJSON[name])
	}

	return sb.String()
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
