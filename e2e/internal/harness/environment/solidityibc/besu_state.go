// SPDX-License-Identifier: Apache-2.0

package solidityibc

import (
	"fmt"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/besumsgs"
)

func validateBesuClientState(raw []byte) error {
	if len(raw) != 5*32 {
		return fmt.Errorf("besu client state has %d bytes, want 160", len(raw))
	}
	schema, err := besumsgs.BindingsMetaData.ParseABI()
	if err != nil {
		return fmt.Errorf("read Besu client state ABI: %w", err)
	}
	inputs := schema.Methods["clientState"].Inputs
	values, err := inputs.Unpack(raw)
	if err != nil {
		return fmt.Errorf("decode Besu client state: %w", err)
	}
	var decoded struct {
		State besumsgs.IBesuLightClientMsgsClientState
	}
	if err := inputs.Copy(&decoded, values); err != nil {
		return fmt.Errorf("read Besu client state: %w", err)
	}
	if decoded.State.LatestHeight.RevisionNumber != 0 || decoded.State.LatestHeight.RevisionHeight == 0 {
		return fmt.Errorf("besu client state has invalid latest height %v", decoded.State.LatestHeight)
	}
	return nil
}
