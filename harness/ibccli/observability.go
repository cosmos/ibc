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

func otelEnvApply(cmd *exec.Cmd, enabled bool, runtimeParts ...string) {
	if !enabled {
		return
	}

	withEnv(cmd, otelServiceRuntimeEnv, strings.Join(runtimeParts, "/"))
}

// defaultOTELConfigPath resolves the repository LGTM-stack OTel SDK config, cli/scripts/otel/ibc-otel.yaml,
// relative to this source file (independent of IBC_BIN).
func defaultOTELConfigPath() string {
	return filepath.Join(filepath.Dir(defaultBinPath("ibc")), "..", "scripts", "otel", "ibc-otel.yaml")
}

type observabilityConfig struct {
	Metrics                 bool   `yaml:"metrics"`
	Type                    string `yaml:"type"`
	SimpleMetricsListenAddr string `yaml:"simpleMetricsListenAddr"`
	OtelFile                string `yaml:"otelFile"`
}
