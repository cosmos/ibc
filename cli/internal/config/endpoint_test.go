// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoteParamsRejectURLSuffixes(t *testing.T) {
	for _, suffix := range []string{"?token=abc", "?", "#section", "#", "/grpc?token=abc", "/grpc#section"} {
		t.Run(suffix, func(t *testing.T) {
			err := (RemoteParams{URL: "https://prover.example.com" + suffix}).Validate()
			require.ErrorContains(t, err, "url: must not contain")
		})
	}
	for _, path := range []string{"/grpc", "/grpc/", "/grpc%3Fv1", "/grpc%23v1"} {
		require.NoError(t, (RemoteParams{URL: "https://prover.example.com" + path}).Validate())
	}
}
