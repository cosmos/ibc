// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/keyfile"
)

func TestSignerSet(t *testing.T) {
	t.Run("setGet", func(t *testing.T) {
		// ARRANGE
		signerSet := NewSet()

		signerA, err := GenerateLocalEd25519Signer()
		require.NoError(t, err)

		signerSet.Set("A", signerA)

		// ACT
		actualSigner, found := signerSet.Get("A")
		missingSigner, missingFound := signerSet.Get("E")

		// ASSERT
		require.True(t, found)
		require.Same(t, signerA, actualSigner)
		require.Nil(t, missingSigner)
		require.False(t, missingFound)
	})
}

func TestNewSignerFromConfigExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	key, err := GenerateLocalEd25519Signer()
	require.NoError(t, err)

	path := filepath.Join(home, "keys", "signer.json")
	require.NoError(t, key.StoreToFile(path))

	loadedSigner, alias, err := NewSignerFromConfig(context.Background(), config.SignerConfig{
		Alias: "local",
		Type:  config.SignerLocal,
		File:  "~/keys/signer.json",
	})

	require.NoError(t, err)
	require.Equal(t, "local", alias)
	require.Equal(t, key.PublicKey(), loadedSigner.PublicKey())
	instrumented, ok := loadedSigner.(*instrumentedSigner)
	require.True(t, ok)
	require.Equal(t, "local", instrumented.alias)
	require.Equal(t, typeLocalEDDSA, instrumented.typ)
}

func TestNewSignerFromConfigRequiresExactFilePath(t *testing.T) {
	key, err := GenerateLocalEd25519Signer()
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, key.StoreToFile(filepath.Join(dir, "signer.json")))

	_, _, err = NewSignerFromConfig(context.Background(), config.SignerConfig{
		Alias: "local",
		Type:  config.SignerLocal,
		File:  filepath.Join(dir, "signer"),
	})
	require.Error(t, err)
}

func TestEVMAddressOf(t *testing.T) {
	key, err := GenerateLocalKey(keyfile.ECDSA)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "deployer.json")
	require.NoError(t, key.StoreToFile(path))
	want, err := PublicKeyToEVMAddress(key.PublicKey())
	require.NoError(t, err)

	got, err := EVMAddressOf(config.SignerConfig{Alias: "d", Type: config.SignerLocal, File: path})
	require.NoError(t, err)
	require.Equal(t, want, got)

	_, err = EVMAddressOf(config.SignerConfig{Alias: "kms", Type: config.SignerRemote})
	require.ErrorContains(t, err, "remote signer")
}

func TestInstrumentedSigner(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	instruments, err := newInstrumentation(provider.Meter("test"))
	require.NoError(t, err)
	previousMetrics := metrics
	metrics = *instruments
	t.Cleanup(func() {
		metrics = previousMetrics
	})

	base, err := GenerateLocalEd25519Signer()
	require.NoError(t, err)
	signer := metricsWrapper("alice", typeLocalEDDSA, base)

	signature, err := signer.Sign(t.Context(), []byte("message"))
	require.NoError(t, err)
	assert.NotEmpty(t, signature)
	assert.Equal(t, base.PublicKey(), signer.PublicKey())

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &data))
	require.Len(t, data.ScopeMetrics, 1)
	require.Len(t, data.ScopeMetrics[0].Metrics, 1)

	histogram, ok := data.ScopeMetrics[0].Metrics[0].Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, histogram.DataPoints, 1)
	assert.Equal(t, uint64(1), histogram.DataPoints[0].Count)
	assert.ElementsMatch(t, []attribute.KeyValue{
		attribute.String("operation", "sign"),
		attribute.String("signer", "alice"),
		attribute.String("type", typeLocalEDDSA),
		attribute.String("result", "ok"),
	}, histogram.DataPoints[0].Attributes.ToSlice())
}
