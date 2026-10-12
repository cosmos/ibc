// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const (
	observabilityTypeOTEL  = "otel"
	otelServiceInstanceEnv = "OTEL_SERVICE_INSTANCE_ID"
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

// otelEnvApply names the process for the service.instance.id OTel attribute,
// which Prometheus stores as the instance label, from its non-empty parts. It
// has no effect unless the process's configuration enables OTel.
func otelEnvApply(cmd *exec.Cmd, instanceParts ...string) {
	parts := slices.DeleteFunc(slices.Clone(instanceParts), func(part string) bool { return part == "" })
	withEnv(cmd, otelServiceInstanceEnv, strings.Join(parts, "/"))
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
