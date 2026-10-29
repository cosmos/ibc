// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/relay/processors"
)

func TestOptionsFromConfigUsesDefaultBatchSize(t *testing.T) {
	opts := OptionsFromConfig(config.Config{}, processors.Route{})

	assert.Equal(t, 25, opts.RecvBatchSize)
	assert.Equal(t, 25, opts.AckBatchSize)
	assert.Equal(t, 25, opts.TimeoutBatchSize)
}
