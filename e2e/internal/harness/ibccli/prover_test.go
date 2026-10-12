// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProverStopKillsAProverThatIgnoresTerm(t *testing.T) {
	script := filepath.Join(t.TempDir(), "prover")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n"+
		"trap '' TERM\n"+
		"echo listening 127.0.0.1:4242\n"+
		"while :; do sleep 1; done\n"), 0o700))
	t.Setenv(proverBinEnv, script)

	prover, err := StartProver(t.Context(), "unused")
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:4242", prover.Address())

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, prover.Stop(ctx), context.DeadlineExceeded)
	select {
	case <-prover.handle.doneCh():
	case <-time.After(5 * time.Second):
		t.Fatal("the prover outlived Stop")
	}
}
