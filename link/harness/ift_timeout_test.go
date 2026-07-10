package harness

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
)

func TestValidateIFTTimeoutRouteRejectsCrossFamilyRoutes(t *testing.T) {
	for _, routeType := range []string{
		wire.RouteEVMToCosmosAttested,
		wire.RouteCosmosToEVMAttested,
	} {
		t.Run(routeType, func(t *testing.T) {
			err := validateIFTTimeoutRoute(wire.Route{
				ID:   "cross-family",
				Type: routeType,
			}, time.Second)
			require.ErrorContains(t, err, "supported only for evmToEvmAttested routes")
		})
	}
}

func TestResolveTimeoutRejectsCrossFamilyRouteBeforeChainAccess(t *testing.T) {
	got, err := (&Session{}).resolveTimeout(context.Background(), wire.Route{
		ID:   "cross-family",
		Type: wire.RouteEVMToCosmosAttested,
	}, time.Second)
	require.ErrorContains(t, err, "supported only for evmToEvmAttested routes")
	require.Zero(t, got)
}

func TestResolveTimeoutAllowsZeroOnCrossFamilyRoutes(t *testing.T) {
	got, err := (&Session{}).resolveTimeout(context.Background(), wire.Route{
		ID:   "cross-family",
		Type: wire.RouteCosmosToEVMAttested,
	}, 0)
	require.NoError(t, err)
	require.Zero(t, got)
}
