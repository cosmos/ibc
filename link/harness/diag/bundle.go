// Package diag collects failure diagnostics for a run.
package diag

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

// Bundle is the failure-diagnostics collector for one run.
type Bundle struct {
	chainLogs  map[string]string // chainID -> captured log text
	heights    map[string]uint64 // chainID -> last observed height
	sutLogs    map[string]string // SUT process name -> captured stderr/combined log text
	statusJSON map[string]string // SUT process name -> captured status API output

	versions   map[string]string // tool/binary -> version string
	configYAML string            // resolved IBC Link config
	topology   string            // topology summary
	deployment string            // rendered deployment metadata (addresses, routes, tx hashes)
}

// NewBundle creates an empty bundle.
func NewBundle() *Bundle {
	return &Bundle{
		chainLogs:  map[string]string{},
		heights:    map[string]uint64{},
		sutLogs:    map[string]string{},
		statusJSON: map[string]string{},
		versions:   map[string]string{},
	}
}

// AddChainLog records log output for a chain.
func (b *Bundle) AddChainLog(chainID, log string) {
	b.chainLogs[chainID] = log
}

// AddSUTLog records output captured from a SUT invocation (the ibc-link Runner's per-run log, or a
// daemon instance's combined output — whichever binary the swap ledger routed to), keyed by process
// name. This is the harness↔SUT boundary's side of the diagnostics: what the black box said on its
// way to a failure.
func (b *Bundle) AddSUTLog(name, log string) {
	b.sutLogs[name] = log
}

// AddStatus records the status-API output captured from a daemon on failure — the better signal for
// pending packets, alongside the on-chain heights. Keyed by daemon name so a restart's second
// instance does not overwrite the first.
func (b *Bundle) AddStatus(name, status string) {
	b.statusJSON[name] = status
}

// SetHeight records the last observed height of a chain.
func (b *Bundle) SetHeight(chainID string, h uint64) {
	b.heights[chainID] = h
}

// SetVersion records a binary/image version (e.g. "anvil", "go-ethereum", "ibc-link").
func (b *Bundle) SetVersion(tool, version string) {
	b.versions[tool] = version
}

// SetConfig records the resolved IBC Link config the harness compiled — it pins exactly what the stub
// consumed.
func (b *Bundle) SetConfig(yaml string) { b.configYAML = yaml }

// SetTopology records a short summary of the topology under test.
func (b *Bundle) SetTopology(summary string) { b.topology = summary }

// SetDeployment records the stub's reported deployment metadata — per-chain fixture addresses and deploy
// tx hashes — rendered into a stable block. These are the packet-trace anchors a failure report needs
// (tx hashes; packet ids/sequences come from the status output and stub logs).
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

// String renders the bundle as a single stably-ordered report.
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

// indent prefixes each non-empty line with two spaces for a nested section.
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
