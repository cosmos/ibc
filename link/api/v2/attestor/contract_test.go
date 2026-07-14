package attestor_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/cosmos/ibc/link/api/v2/attestor"
)

func TestAttestorAPIContract(t *testing.T) {
	t.Parallel()

	require.Equal(t, "attestor.proto", attestor.File_attestor_proto.Path())
	require.Equal(t, protoreflect.FullName("ibc.v2.attestor"), attestor.File_attestor_proto.Package())
	require.Equal(t, "ibc.v2.attestor.AttestationService", attestor.AttestationServiceName)
	require.Equal(
		t,
		"/ibc.v2.attestor.AttestationService/LatestAttestableHeight",
		attestor.AttestationServiceLatestAttestableHeightProcedure,
	)

	service := attestor.File_attestor_proto.Services().ByName("AttestationService")
	require.NotNil(t, service)
	require.Equal(t, protoreflect.FullName("ibc.v2.attestor.AttestationService"), service.FullName())

	method := service.Methods().ByName("LatestAttestableHeight")
	require.NotNil(t, method)
	require.Equal(
		t,
		protoreflect.FullName("ibc.v2.attestor.AttestationService.LatestAttestableHeight"),
		method.FullName(),
	)
	require.Equal(t, protoreflect.FullName("ibc.v2.attestor.LatestAttestableHeightRequest"), method.Input().FullName())
	require.Equal(t, protoreflect.FullName("ibc.v2.attestor.LatestAttestableHeightResponse"), method.Output().FullName())
	require.False(t, method.IsStreamingClient())
	require.False(t, method.IsStreamingServer())

	requestField := method.Input().Fields().ByName("attestor")
	require.NotNil(t, requestField)
	require.Equal(t, protoreflect.FieldNumber(1), requestField.Number())
	require.Equal(t, "attestor", requestField.JSONName())
	require.Equal(t, protoreflect.StringKind, requestField.Kind())

	responseField := method.Output().Fields().ByName("height")
	require.NotNil(t, responseField)
	require.Equal(t, protoreflect.FieldNumber(1), responseField.Number())
	require.Equal(t, "height", responseField.JSONName())
	require.Equal(t, protoreflect.Uint64Kind, responseField.Kind())

	path, _ := attestor.NewAttestationServiceHandler(attestor.UnimplementedAttestationServiceHandler{})
	require.Equal(t, "/ibc.v2.attestor.AttestationService/", path)
}

func TestAttestorProcessReadinessContract(t *testing.T) {
	t.Parallel()

	require.Equal(t, "ready", attestor.ProcessReadinessEvent)

	encoded, err := json.Marshal(attestor.ProcessReadiness{
		Event: attestor.ProcessReadinessEvent,
		HTTP:  "127.0.0.1:3000",
	})
	require.NoError(t, err)
	require.Equal(t, `{"event":"ready","http":"127.0.0.1:3000"}`, string(encoded))
}
