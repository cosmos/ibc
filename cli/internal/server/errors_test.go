// SPDX-License-Identifier: Apache-2.0

package server

import (
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/cosmos/ibc/cli/internal/service/attestor"
	"github.com/cosmos/ibc/cli/internal/service/relayer"
)

func TestConnectError(t *testing.T) {
	for name, tt := range map[string]struct {
		err   error
		codes []errorCode
		want  connect.Code
	}{
		"attestorNotFound":           {attestor.ErrNotFound, attestorCodes, connect.CodeNotFound},
		"attestorCommitmentNotFound": {attestor.ErrCommitmentNotFound, attestorCodes, connect.CodeNotFound},
		"attestorInvalidInput":       {attestor.ErrInvalidInput, attestorCodes, connect.CodeInvalidArgument},
		"attestorNotFinalized":       {attestor.ErrNotFinalized, attestorCodes, connect.CodeFailedPrecondition},
		"attestorReceiptExists":      {attestor.ErrReceiptExists, attestorCodes, connect.CodeFailedPrecondition},
		"relayerInvalidInput":        {relayer.ErrInvalidInput, relayerCodes, connect.CodeInvalidArgument},
		"relayerFailedPrecondition":  {relayer.ErrFailedPrecondition, relayerCodes, connect.CodeFailedPrecondition},
		"relayerNotFound":            {relayer.ErrNotFound, relayerCodes, connect.CodeNotFound},
		"wrapped":                    {errors.Wrap(attestor.ErrNotFinalized, "height 9"), attestorCodes, connect.CodeFailedPrecondition},
		"unmapped":                   {errors.New("rpc down"), attestorCodes, connect.CodeInternal},
	} {
		t.Run(name, func(t *testing.T) {
			got := connectError(slog.Default(), "Method", tt.err, tt.codes)
			assert.Equal(t, tt.want, connect.CodeOf(got))
		})
	}

	t.Run("unmappedHidesTheCause", func(t *testing.T) {
		got := connectError(slog.Default(), "Method", errors.New("rpc down"), relayerCodes)
		assert.Equal(t, errInternal, got)
	})
}
