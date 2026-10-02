// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	observabilityTypeOTEL = "otel"
	otelServiceRuntimeEnv = "OTEL_SERVICE_RUNTIME"
)

type observabilityContextKey struct{}

// WithObservability controls observability for IBC CLI processes started with ctx.
func WithObservability(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, observabilityContextKey{}, enabled)
}

// ObservabilityEnabled reports whether observability is enabled in ctx.
func ObservabilityEnabled(ctx context.Context) bool {
	enabled, _ := ctx.Value(observabilityContextKey{}).(bool)
	return enabled
}

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

func defaultOTELConfigPath() string {
	return filepath.Join(filepath.Dir(defaultBinPath("ibc")), "..", "scripts", "otel", "ibc-otel.yaml")
}

type observabilityConfig struct {
	Metrics                 bool   `yaml:"metrics"`
	Type                    string `yaml:"type"`
	SimpleMetricsListenAddr string `yaml:"simpleMetricsListenAddr"`
	OtelFile                string `yaml:"otelFile"`
}
