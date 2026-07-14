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

func Output(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func MissingContainer(err error) bool {
	return strings.Contains(err.Error(), "No such container")
}

func Safe(in string) string {
	out := nameRe.ReplaceAllString(strings.ToLower(in), "-")
	out = strings.Trim(out, "-_.")
	if out == "" {
		return "run"
	}
	return out
}

// RunLabels and NamePrefix must stay compatible with scripts/clean-e2e.sh.
func RunLabels(runID string) []string {
	return []string{"--label", "ibc-link-e2e=true", "--label", "ibc-link-run=" + runID}
}

func NamePrefix(runID, chainID string) string {
	return "ibc-link-e2e-" + Safe(runID) + "-" + Safe(chainID)
}
