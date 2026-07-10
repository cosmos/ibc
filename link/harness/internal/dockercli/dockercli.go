// Package dockercli contains thin helpers around the Docker CLI used by managed chain providers.
package dockercli

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var nameRe = regexp.MustCompile(`[^a-z0-9_.-]+`)

// Output runs docker with args and returns its combined stdout/stderr.
func Output(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// Missing reports whether err is Docker's usual wording for an absent container or network.
func Missing(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "No such container") ||
		strings.Contains(s, "No such network") ||
		(strings.Contains(s, "network ") && strings.Contains(s, " not found"))
}

// Safe sanitizes a caller-provided value for use in Docker names.
func Safe(in string) string {
	out := nameRe.ReplaceAllString(strings.ToLower(in), "-")
	out = strings.Trim(out, "-_.")
	if out == "" {
		return "run"
	}
	return out
}

// RunLabels is the `--label` args every harness-launched container carries: `ibc-link-e2e=true` finds
// all of them, `ibc-link-run=<runID>` scopes one run. The label values and the NamePrefix form are a
// contract with scripts/clean-e2e.sh, which sweeps leaked containers/networks by either.
func RunLabels(runID string) []string {
	return []string{"--label", "ibc-link-e2e=true", "--label", "ibc-link-run=" + runID}
}

// NamePrefix is the canonical name prefix for one chain's containers, sanitized for Docker; a launcher
// uses it bare or appends a role suffix (e.g. "-anvil", "-generate").
func NamePrefix(runID, chainID string) string {
	return "ibc-link-e2e-" + Safe(runID) + "-" + Safe(chainID)
}
