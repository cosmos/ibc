// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/proofgen"
)

type stubParams struct {
	ProverURL string `yaml:"proverUrl"`
}

// stubFactory stands in for a custom light client's factory.
type stubFactory struct {
	requireParams bool
}

func (f stubFactory) ValidateParams(params *proofgen.RawParams) error {
	if !f.requireParams {
		return nil
	}

	var p stubParams
	if err := params.Decode(&p); err != nil {
		return err
	}

	if p.ProverURL == "" {
		return errRequired
	}

	return nil
}

func (f stubFactory) New(
	context.Context, proofgen.Deps, proofgen.ClientEnd, proofgen.ClientEnd,
) (proofgen.ProofGenerator, error) {
	return nil, nil
}

var errRequired = errorString("proverUrl required")

type errorString string

func (e errorString) Error() string { return string(e) }

func loadSample(t *testing.T) Config {
	t.Helper()

	config, err := LoadFromFile(filepath.Join("testdata", "sample.yml"), true, true)
	require.NoError(t, err)

	return config
}

func TestValidateClients(t *testing.T) {
	t.Run("registered type passes", func(t *testing.T) {
		config := loadSample(t)

		reg := proofgen.NewRegistry()
		require.NoError(t, reg.Register(string(ClientTypeAttestation), stubFactory{}))

		require.NoError(t, config.ValidateClients(reg))
	})

	t.Run("unregistered type fails and lists what is registered", func(t *testing.T) {
		config := loadSample(t)
		config.Relayer.Connections[0].ClientA.Type = "tendermint"

		reg := proofgen.NewRegistry()
		require.NoError(t, reg.Register(string(ClientTypeAttestation), stubFactory{}))

		err := config.ValidateClients(reg)
		require.ErrorContains(t, err, `unknown client type "tendermint"`)
		require.ErrorContains(t, err, "attestation")
	})

	t.Run("arbitrary type name resolves once registered", func(t *testing.T) {
		config := loadSample(t)
		for i := range config.Relayer.Connections {
			config.Relayer.Connections[i].ClientA.Type = "myclient"
			config.Relayer.Connections[i].ClientB.Type = "myclient"
		}

		reg := proofgen.NewRegistry()
		require.NoError(t, reg.Register("myclient", stubFactory{}))

		require.NoError(t, config.ValidateClients(reg))
	})

	t.Run("factory params error is surfaced", func(t *testing.T) {
		config := loadSample(t)

		reg := proofgen.NewRegistry()
		require.NoError(t, reg.Register(string(ClientTypeAttestation), stubFactory{requireParams: true}))

		err := config.ValidateClients(reg)
		require.ErrorContains(t, err, ".params")
		require.ErrorContains(t, err, "proverUrl required")
	})

	t.Run("nil registry is an error, not a silent pass", func(t *testing.T) {
		config := loadSample(t)

		require.Error(t, config.ValidateClients(nil))
	})
}

// A misspelled params key must fail rather than silently becoming a zero
// value: the top-level decode's DisallowUnknownField cannot see inside a
// captured params block, so RawParams.Decode has to re-apply it.
func TestRawParamsRejectsUnknownField(t *testing.T) {
	params := proofgen.NewRawParams([]byte("proverURL: https://example.com\n"))

	var p stubParams
	require.ErrorContains(t, params.Decode(&p), "proverURL")
	require.Empty(t, p.ProverURL)
}

func TestRawParamsRoundTrip(t *testing.T) {
	params := proofgen.NewRawParams([]byte("proverUrl: https://example.com\n"))

	var p stubParams
	require.NoError(t, params.Decode(&p))
	require.Equal(t, "https://example.com", p.ProverURL)
}

func TestRawParamsEmptyDecodeIsNoOp(t *testing.T) {
	var params *proofgen.RawParams

	var p stubParams
	require.NoError(t, params.Decode(&p))
	require.True(t, params.IsEmpty())
}
