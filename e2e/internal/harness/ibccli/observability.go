// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	observabilityTypeOTEL = "otel"
	otelServiceRuntimeEnv = "OTEL_SERVICE_RUNTIME"
)

func otelFileConfig(enabled bool) observabilityConfig {
	if !enabled {
		return observabilityConfig{}
	}

	return observabilityConfig{
		Metrics:  true,
		Type:     observabilityTypeOTEL,
		OtelFile: defaultOTELConfigPath(),
	}
}

// otelEnvApply names the process for the service.runtime OTel attribute. It
// has no effect unless the process's configuration enables OTel.
func otelEnvApply(cmd *exec.Cmd, runtimeParts ...string) {
	withEnv(cmd, otelServiceRuntimeEnv, strings.Join(runtimeParts, "/"))
}

// defaultOTELConfigPath resolves the repository LGTM-stack OTel SDK config, cli/scripts/otel/ibc-otel.yaml,
// relative to this source file (independent of IBC_BIN).
func defaultOTELConfigPath() string {
	return filepath.Join(filepath.Dir(defaultBinPath("ibc")), "..", "scripts", "otel", "ibc-otel.yaml")
}

type observabilityConfig struct {
	Metrics  bool   `yaml:"metrics"`
	Type     string `yaml:"type"`
	OtelFile string `yaml:"otelFile"`
}
