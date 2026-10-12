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
	client := &EVMClient{accounts: map[common.Address]*accountState{}}
	sender := common.HexToAddress("0x01")
	gate := client.account(sender).gate
	gate <- struct{}{}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	err := acquireTx(ctx, client.account(sender).gate)

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestTransactionSerializationIsPerSender(t *testing.T) {
	client := &EVMClient{accounts: map[common.Address]*accountState{}}
	client.account(common.HexToAddress("0x01")).gate <- struct{}{}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	require.NoError(t, acquireTx(ctx, client.account(common.HexToAddress("0x02")).gate))
}

func TestNormalizeWaitErrorPreservesDeadlineAndOperation(t *testing.T) {
	err := normalizeWaitError(
		context.Background(),
		fmt.Errorf("read pending nonce: %w", os.ErrDeadlineExceeded),
	)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "read pending nonce")
}
