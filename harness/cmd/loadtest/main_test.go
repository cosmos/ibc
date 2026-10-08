// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Invalid values are rejected at parse time, before any environment starts.
func TestRejectsInvalidFlags(t *testing.T) {
	for name, args := range map[string][]string{
		"one wallet":         {"relayer", "--wallets", "1"},
		"zero per-block":     {"relayer", "--per-block", "0"},
		"zero packets":       {"relayer", "--packets", "0"},
		"bad direction":      {"relayer", "--directions", "up"},
		"bad preset":         {"relayer", "--preset", "huge"},
		"stray argument":     {"relayer", "extra"},
		"zero timeout":       {"relayer", "--timeout", "0s"},
		"zero rps":           {"attestor", "--packets-rps", "0"},
		"negative duration":  {"attestor", "--duration", "-1s"},
		"error rate above 1": {"attestor", "--max-error-rate", "1.5"},
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

func TestHelpExitsZero(t *testing.T) {
	assert.Equal(t, exitOK, run([]string{"relayer", "-h"}, &bytes.Buffer{}, &bytes.Buffer{}))
	assert.Equal(t, exitOK, run([]string{"help"}, &bytes.Buffer{}, &bytes.Buffer{}))
}
