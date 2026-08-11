// SPDX-License-Identifier: Apache-2.0

package evm

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestRequireEOARejectsZeroBeforeRPC(t *testing.T) {
	err := (&EVMClient{}).RequireEOA(t.Context(), common.Address{})
	require.ErrorContains(t, err, "EOA address is zero")
}

func TestTransactionSerializationObservesContext(t *testing.T) {
	client := &EVMClient{txGate: make(chan struct{}, 1)}
	client.txGate <- struct{}{}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	err := client.acquireTx(ctx)

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestNormalizeWaitErrorPreservesDeadlineAndOperation(t *testing.T) {
	err := normalizeWaitError(
		context.Background(),
		fmt.Errorf("read pending nonce: %w", os.ErrDeadlineExceeded),
	)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "read pending nonce")
}
