// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cosmos/ibc/e2e/internal/loadtest/relayerload"
	"github.com/cosmos/ibc/e2e/internal/loadtest/session"
)

// Invalid values are rejected at parse time, before any environment starts.
func TestRejectsInvalidFlags(t *testing.T) {
	for name, args := range map[string][]string{
		"zero wallets":       {"relayer", "--wallets", "0"},
		"zero per-block":     {"relayer", "--per-block", "0"},
		"zero packets":       {"relayer", "--packets", "0"},
		"bad direction":      {"relayer", "--directions", "up"},
		"bad light client":   {"relayer", "--light-client", "tendermint"},
		"qbft on anvil":      {"relayer", "--light-client", "besu-qbft"},
		"stray argument":     {"relayer", "extra"},
		"zero timeout":       {"relayer", "--timeout", "0s"},
		"zero rps":           {"attestor", "--packets-rps", "0"},
		"negative duration":  {"attestor", "--duration", "-1s"},
		"error rate above 1": {"attestor", "--max-error-rate", "1.5"},
		"NaN error rate":     {"attestor", "--max-error-rate", "NaN"},
		"zero selected rps":  {"attestor", "--phases", "state", "--state-rps", "0"},
		"bad phase":          {"attestor", "--phases", "packets,blocks"},
		"unknown flag":       {"attestor", "--rps", "5"},
		"unknown scenario":   {"prover"},
	} {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			assert.Equal(t, exitUsage, run(args, &bytes.Buffer{}, &stderr))
			assert.NotEmpty(t, stderr.String(), "says why")
		})
	}
}

// A rate only matters for a phase that runs: this run gets past the phase
// checks and stops at the provider.
func TestIgnoresTheRateOfAnUnselectedPhase(t *testing.T) {
	var stderr bytes.Buffer
	args := []string{"attestor", "--phases", "state", "--packets-rps", "0", "--provider", "none"}
	assert.Equal(t, exitUsage, run(args, &bytes.Buffer{}, &stderr))
	assert.Contains(t, stderr.String(), "provider")
}

func TestExitCodes(t *testing.T) {
	assert.Equal(t, exitOK, exitCode(nil))
	assert.Equal(t, exitFailed, exitCode(errors.New("relay stalled")))
	assert.Equal(t, exitInconclusive, exitCode(fmt.Errorf("%w: a-to-b", relayerload.ErrLoadNotDelivered)))
}

// Keeping the environment changes only teardown, not the exit code.
func TestFinishKeepsTheExitCode(t *testing.T) {
	inconclusive := fmt.Errorf("%w: a-to-b", relayerload.ErrLoadNotDelivered)
	for _, keep := range []bool{true, false} {
		assert.Equal(t, exitInconclusive, finish(t.Context(), &session.Session{}, inconclusive, keep))
	}
	assert.Equal(t, exitFailed, finish(t.Context(), nil, errors.New("setup failed"), true))
}

func TestHelpExitsZero(t *testing.T) {
	assert.Equal(t, exitOK, run([]string{"relayer", "-h"}, &bytes.Buffer{}, &bytes.Buffer{}))
	assert.Equal(t, exitOK, run([]string{"help"}, &bytes.Buffer{}, &bytes.Buffer{}))
}
