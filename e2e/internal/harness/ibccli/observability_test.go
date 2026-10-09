// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOTelFileConfig(t *testing.T) {
	require.Equal(t, observabilityConfig{
		Metrics:  true,
		Type:     observabilityTypeOTEL,
		OtelFile: defaultOTELConfigPath(),
	}, otelFileConfig(true))
	require.Empty(t, otelFileConfig(false))
}

func TestOTelEnvApplyKeepsTheInheritedEnvironment(t *testing.T) {
	cmd := exec.Command("unused")
	otelEnvApply(cmd, "attestor", "test")
	require.Contains(t, cmd.Env, otelServiceRuntimeEnv+"=attestor/test")
	require.Contains(t, cmd.Env, "PATH="+os.Getenv("PATH"))
}
