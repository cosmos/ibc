// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnabledObservability(t *testing.T) {
	require.Equal(t, observabilityConfig{
		Metrics:  true,
		Type:     observabilityTypeOTEL,
		OtelFile: defaultOTELConfigPath(),
	}, otelFileConfig(true))

	cmd := exec.Command("unused")
	otelEnvApply(cmd, true, "relayer", "test")
	require.Contains(t, cmd.Env, otelServiceRuntimeEnv+"=relayer/test")
	require.Contains(t, cmd.Env, "PATH="+os.Getenv("PATH"))
}

func TestDisabledObservability(t *testing.T) {
	cmd := exec.Command("unused")
	otelEnvApply(cmd, false, "relayer")

	require.Empty(t, otelFileConfig(false))
	require.Nil(t, cmd.Env)
}

func TestObservabilityContext(t *testing.T) {
	require.False(t, ObservabilityEnabled(context.Background()))
	require.True(t, ObservabilityEnabled(WithObservability(context.Background(), true)))
}
