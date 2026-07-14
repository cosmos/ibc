package relayer_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/cosmos/ibc/link/api/v2/relayer"
)

func TestRelayerAPIContract(t *testing.T) {
	t.Parallel()

	require.Equal(t, "relayer.proto", relayer.File_relayer_proto.Path())
	require.Equal(t, protoreflect.FullName("ibc.v2.relayer"), relayer.File_relayer_proto.Package())
	require.Equal(t, "ibc.v2.relayer.RelayerApiService", relayer.RelayerApiServiceName)
	require.Equal(
		t,
		"/ibc.v2.relayer.RelayerApiService/Relay",
		relayer.RelayerApiServiceRelayProcedure,
	)

	service := relayer.File_relayer_proto.Services().ByName("RelayerApiService")
	require.NotNil(t, service)
	require.Equal(t, protoreflect.FullName("ibc.v2.relayer.RelayerApiService"), service.FullName())

	method := service.Methods().ByName("Relay")
	require.NotNil(t, method)
	require.Equal(t, protoreflect.FullName("ibc.v2.relayer.RelayerApiService.Relay"), method.FullName())
	require.Equal(t, protoreflect.FullName("ibc.v2.relayer.RelayRequest"), method.Input().FullName())
	require.Equal(t, protoreflect.FullName("ibc.v2.relayer.RelayResponse"), method.Output().FullName())
	require.False(t, method.IsStreamingClient())
	require.False(t, method.IsStreamingServer())

	txHashField := method.Input().Fields().ByName("tx_hash")
	require.NotNil(t, txHashField)
	require.Equal(t, protoreflect.FieldNumber(1), txHashField.Number())
	require.Equal(t, "txHash", txHashField.JSONName())
	require.Equal(t, protoreflect.StringKind, txHashField.Kind())

	chainIDField := method.Input().Fields().ByName("chain_id")
	require.NotNil(t, chainIDField)
	require.Equal(t, protoreflect.FieldNumber(2), chainIDField.Number())
	require.Equal(t, "chainId", chainIDField.JSONName())
	require.Equal(t, protoreflect.StringKind, chainIDField.Kind())
	require.Zero(t, method.Output().Fields().Len())

	path, _ := relayer.NewRelayerApiServiceHandler(relayer.UnimplementedRelayerApiServiceHandler{})
	require.Equal(t, "/ibc.v2.relayer.RelayerApiService/", path)
}
