// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/cli/internal/config"
)

func TestApplyClearOnStart(t *testing.T) {
	value := flagRelayerClearOnStart
	t.Cleanup(func() { flagRelayerClearOnStart = value })

	configured := false
	cfg := config.Config{Relayer: config.RelayerConfig{ClearOnStart: &configured}}

	applyClearOnStart(cmdRelayerRun, &cfg)
	require.False(t, *cfg.Relayer.ClearOnStart, "an unpassed flag must not mask relayer.clearOnStart")

	require.NoError(t, cmdRelayerRun.ParseFlags([]string{"--" + flagClearOnStart + "=true"}))
	applyClearOnStart(cmdRelayerRun, &cfg)
	require.True(t, *cfg.Relayer.ClearOnStart)
}
