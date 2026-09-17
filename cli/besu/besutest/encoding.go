// SPDX-License-Identifier: Apache-2.0

package besutest

import (
	"fmt"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
)

var messageBindings = besumsgs.NewBindings()

// EncodeClientState produces getClientState() output, for tests and fakes.
func EncodeClientState(state besumsgs.IBesuLightClientMsgsClientState) ([]byte, error) {
	data, err := messageBindings.TryPackClientState(state)
	if err != nil {
		return nil, fmt.Errorf("encode client state: %w", err)
	}
	return data[4:], nil
}
