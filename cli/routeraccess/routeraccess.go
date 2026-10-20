// SPDX-License-Identifier: Apache-2.0

// Package routeraccess owns which ICS26Router methods a deployment opens to
// everyone through its AccessManager.
package routeraccess

import (
	"fmt"
	"math"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
)

// PublicRole is OpenZeppelin AccessManager's PUBLIC_ROLE.
const PublicRole uint64 = math.MaxUint64

// publicRelayingMethods are opened to PublicRole so any relayer EOA can
// submit packets and client updates.
var publicRelayingMethods = []string{
	"recvPacket",
	"ackPacket",
	"timeoutPacket",
	"updateClient",
	"multicall",
	"submitMisbehaviour",
}

// PublicRelayingSelectors returns the selectors of the router methods a
// deployment binds to PublicRole.
func PublicRelayingSelectors() ([][4]byte, error) {
	routerABI, err := ics26router.ContractMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	selectors := make([][4]byte, 0, len(publicRelayingMethods))
	for _, name := range publicRelayingMethods {
		method, ok := routerABI.Methods[name]
		if !ok {
			return nil, fmt.Errorf("router ABI has no method %q", name)
		}
		selectors = append(selectors, [4]byte(method.ID))
	}
	return selectors, nil
}
