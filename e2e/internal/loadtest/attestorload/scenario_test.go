// SPDX-License-Identifier: Apache-2.0

package attestorload

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/e2e/internal/loadtest/simpleload"
)

func validConfig() Config {
	return Config{
		Packets:      simpleload.Spec{Duration: time.Second, RatePerSecond: 10},
		State:        simpleload.Spec{Duration: time.Second, RatePerSecond: 10},
		Phases:       []string{PhasePackets, PhaseState},
		MaxErrorRate: 0.01,
	}
}

// A rate only matters for a phase that runs.
func TestValidateIgnoresAnUnselectedPhase(t *testing.T) {
	cfg := validConfig()
	cfg.Phases = []string{PhaseState}
	cfg.Packets.RatePerSecond = 0
	require.NoError(t, cfg.Validate())
}

func TestValidateRejects(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"negative duration times negative rate": func(c *Config) {
			c.State = simpleload.Spec{Duration: -time.Second, RatePerSecond: -100}
		},
		"zero duration":  func(c *Config) { c.Packets.Duration = 0 },
		"unknown phase":  func(c *Config) { c.Phases = []string{"blocks"} },
		"no phases":      func(c *Config) { c.Phases = nil },
		"error rate 1.5": func(c *Config) { c.MaxErrorRate = 1.5 },
		"no whole call":  func(c *Config) { c.State = simpleload.Spec{Duration: 500 * time.Millisecond, RatePerSecond: 1} },
	} {
		cfg := validConfig()
		change(&cfg)
		assert.Error(t, cfg.Validate(), name)
	}
}
