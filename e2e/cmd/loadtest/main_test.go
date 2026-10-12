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
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"zero wallets":              {[]string{"relayer", "--wallets", "0"}, "wallet count"},
		"zero per-block":            {[]string{"relayer", "--per-block", "0"}, "transfers per block"},
		"zero packets":              {[]string{"relayer", "--packets", "0"}, "positive packet count or duration"},
		"packets and duration":      {[]string{"relayer", "--packets", "10", "--duration", "1m"}, "not both"},
		"negative relayer duration": {[]string{"relayer", "--duration", "-1m"}, "positive packet count or duration"},
		"zero batch":                {[]string{"relayer", "--batch", "0"}, "batch size must be positive"},
		"batch not dividing":        {[]string{"relayer", "--per-block", "10", "--batch", "3"}, "multiple of the batch"},
		"bad direction":             {[]string{"relayer", "--directions", "up"}, "unknown direction"},
		"bad light client":          {[]string{"relayer", "--light-client", "tendermint"}, "unknown light client"},
		"qbft on anvil":             {[]string{"relayer", "--light-client", "besu-qbft"}, "need besu"},
		"stray argument":            {[]string{"relayer", "extra"}, "unexpected arguments"},
		"zero timeout":              {[]string{"relayer", "--timeout", "0s"}, "--timeout must be positive"},
		"zero rps":                  {[]string{"attestor", "--packets-rps", "0"}, "rate 0 per second"},
		"negative duration":         {[]string{"attestor", "--duration", "-1s"}, "duration -1s"},
		"error rate above 1":        {[]string{"attestor", "--max-error-rate", "1.5"}, "max error rate 1.5"},
		"NaN error rate":            {[]string{"attestor", "--max-error-rate", "NaN"}, "max error rate NaN"},
		"zero selected rps":         {[]string{"attestor", "--phases", "state", "--state-rps", "0"}, "state phase"},
		"bad phase":                 {[]string{"attestor", "--phases", "packets,blocks"}, `unknown phase "blocks"`},
		"no whole call": {
			[]string{"attestor", "--phases", "state", "--duration", "500ms", "--state-rps", "1"}, "sends no calls",
		},
		"unknown flag":     {[]string{"attestor", "--rps", "5"}, "flag provided but not defined"},
		"unknown scenario": {[]string{"prover"}, `unknown scenario "prover"`},
	} {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			assert.Equal(t, exitUsage, run(tc.args, &bytes.Buffer{}, &stderr))
			assert.Contains(t, stderr.String(), tc.want, "says why")
		})
	}
}

func TestExitCodes(t *testing.T) {
	assert.Equal(t, exitOK, exitCode(nil))
	assert.Equal(t, exitFailed, exitCode(errors.New("relay stalled")))
	inconclusive := fmt.Errorf("%w: a-to-b", relayerload.ErrLoadNotDelivered)
	assert.Equal(t, exitInconclusive, exitCode(inconclusive))
	assert.Equal(t, exitFailed, exitCode(errors.Join(inconclusive, errReportNotWritten)),
		"an unwritten report fails an inconclusive run")
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
