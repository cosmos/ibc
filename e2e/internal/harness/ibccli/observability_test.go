// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The keys are the CLI's observability configuration (cli/internal/config),
// and the file is the repository's OTel SDK configuration.
func TestOTelFileConfig(t *testing.T) {
	encoded, err := yaml.Marshal(otelFileConfig(true))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, yaml.Unmarshal(encoded, &got))
	file, ok := got["otelFile"].(string)
	require.True(t, ok)
	require.Equal(t, map[string]any{"metrics": true, "type": "otel", "otelFile": file}, got)
	require.FileExists(t, file)
	require.Empty(t, otelFileConfig(false))
}

func TestOTelEnvApplyKeepsTheInheritedEnvironment(t *testing.T) {
	cmd := exec.Command("unused")
	otelEnvApply(cmd, "run-1", "attestor", "test")
	require.Contains(t, cmd.Env, otelServiceInstanceEnv+"=run-1/attestor/test")
	require.Contains(t, cmd.Env, "PATH="+os.Getenv("PATH"))
}
