// SPDX-License-Identifier: Apache-2.0

package besu_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"

	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
)

func TestPacketCommitmentPath(t *testing.T) {
	path := hostv2.PacketCommitmentKey("besu-chain-b", 1)
	assert.Equal(t, "626573752d636861696e2d62010000000000000001", common.Bytes2Hex(path))
}
