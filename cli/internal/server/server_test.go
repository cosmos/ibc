// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"

	"github.com/cosmos/ibc/cli/api/v2/attestor"
	"github.com/cosmos/ibc/cli/api/v2/relayer"
	"github.com/cosmos/ibc/cli/internal/config"
	attestorservice "github.com/cosmos/ibc/cli/internal/service/attestor"
	relayerservice "github.com/cosmos/ibc/cli/internal/service/relayer"
)

func TestServerHealthCheck(t *testing.T) {
	srv := New("127.0.0.1:0", false, false)
	registerHealthTestServices(t, srv, func(context.Context) error { return nil })
	conn := startHealthTestServer(t, srv)
	client := healthpb.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	for _, service := range []string{"", grpchealth.HealthV1ServiceName, relayer.RelayerApiServiceName, attestor.AttestationServiceName} {
		res, err := client.Check(ctx, &healthpb.HealthCheckRequest{Service: service})
		require.NoError(t, err)
		assert.Equal(t, healthpb.HealthCheckResponse_SERVING, res.GetStatus(), service)
	}
	_, err := client.Check(ctx, &healthpb.HealthCheckRequest{Service: "unknown.service"})
	require.Equal(t, codes.NotFound, status.Code(err))

	stream, err := client.Watch(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	_, err = stream.Recv()
	require.Equal(t, codes.Unimplemented, status.Code(err))

	connectClient := connect.NewClient[healthpb.HealthCheckRequest, healthpb.HealthCheckResponse](
		http.DefaultClient, "http://"+conn.Target()+healthpb.Health_Check_FullMethodName,
	)
	res, err := connectClient.CallUnary(ctx, connect.NewRequest(&healthpb.HealthCheckRequest{}))
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, res.Msg.GetStatus())
}

func TestServerHealthReflection(t *testing.T) {
	srv := New("127.0.0.1:0", true, false)
	conn := startHealthTestServer(t, srv)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	health, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, health.GetStatus())

	stream, err := reflectionpb.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{},
	}))
	res, err := stream.Recv()
	require.NoError(t, err)
	var names []string
	for _, service := range res.GetListServicesResponse().GetService() {
		names = append(names, service.GetName())
	}
	assert.Contains(t, names, grpchealth.HealthV1ServiceName)

	require.NoError(t, stream.Send(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: grpchealth.HealthV1ServiceName,
		},
	}))
	res, err = stream.Recv()
	require.NoError(t, err)
	assert.NotEmpty(t, res.GetFileDescriptorResponse().GetFileDescriptorProto())
}

func TestServerReadinessOnDemand(t *testing.T) {
	srv := New("127.0.0.1:0", false, false)
	var available atomic.Bool
	var calls atomic.Int32
	registerHealthTestServices(t, srv, func(context.Context) error {
		calls.Add(1)
		if !available.Load() {
			return errors.New("database unavailable")
		}
		return nil
	})
	conn := startHealthTestServer(t, srv)
	client := healthpb.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	check := func(service string, want healthpb.HealthCheckResponse_ServingStatus) {
		t.Helper()
		res, err := client.Check(ctx, &healthpb.HealthCheckRequest{Service: service})
		require.NoError(t, err)
		assert.Equal(t, want, res.GetStatus(), service)
	}

	assert.Zero(t, calls.Load())
	check(grpchealth.HealthV1ServiceName, healthpb.HealthCheckResponse_SERVING)
	check(attestor.AttestationServiceName, healthpb.HealthCheckResponse_SERVING)
	assert.Zero(t, calls.Load())
	for i, want := range []healthpb.HealthCheckResponse_ServingStatus{
		healthpb.HealthCheckResponse_NOT_SERVING,
		healthpb.HealthCheckResponse_SERVING,
		healthpb.HealthCheckResponse_NOT_SERVING,
	} {
		available.Store(want == healthpb.HealthCheckResponse_SERVING)
		check("", want)
		check(relayer.RelayerApiServiceName, want)
		check(grpchealth.HealthV1ServiceName, healthpb.HealthCheckResponse_SERVING)
		check(attestor.AttestationServiceName, healthpb.HealthCheckResponse_SERVING)
		assert.EqualValues(t, (i+1)*2, calls.Load())
	}
}

func TestServerReadinessTimeout(t *testing.T) {
	srv := New("127.0.0.1:0", false, false)
	started := make(chan struct{})
	registerHealthTestServices(t, srv, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return nil // A late success must still fail the readiness deadline.
	})
	conn := startHealthTestServer(t, srv)
	client := healthpb.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	type result struct {
		response *healthpb.HealthCheckResponse
		err      error
	}
	done := make(chan result, 1)
	go func() {
		res, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
		done <- result{res, err}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("readiness check did not start")
	}

	liveCtx, liveCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer liveCancel()
	res, err := client.Check(liveCtx, &healthpb.HealthCheckRequest{Service: grpchealth.HealthV1ServiceName})
	require.NoError(t, err)
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, res.GetStatus())

	outcome := <-done
	require.NoError(t, outcome.err)
	assert.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, outcome.response.GetStatus())
	assert.NoError(t, ctx.Err())
}

func TestServerReadinessCancellation(t *testing.T) {
	srv := New("127.0.0.1:0", false, false)
	started := make(chan struct{})
	canceled := make(chan error, 1)
	registerHealthTestServices(t, srv, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		canceled <- ctx.Err()
		return ctx.Err()
	})
	conn := startHealthTestServer(t, srv)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	requestCtx, requestCancel := context.WithCancel(ctx)
	defer requestCancel()

	go func() {
		select {
		case <-started:
			requestCancel()
		case <-ctx.Done():
		}
	}()
	_, err := healthpb.NewHealthClient(conn).Check(requestCtx, &healthpb.HealthCheckRequest{})
	require.Equal(t, codes.Canceled, status.Code(err))
	select {
	case err := <-canceled:
		require.ErrorIs(t, err, context.Canceled)
	case <-ctx.Done():
		t.Fatal("database probe did not receive cancellation")
	}
}

func TestServerHealthLifecycle(t *testing.T) {
	srv := New("127.0.0.1:0", false, false)
	registerHealthTestServices(t, srv, func(context.Context) error { return nil })
	check := func(want grpchealth.Status) {
		t.Helper()
		for _, service := range []string{"", grpchealth.HealthV1ServiceName, relayer.RelayerApiServiceName, attestor.AttestationServiceName} {
			res, err := srv.health.Check(t.Context(), &grpchealth.CheckRequest{Service: service})
			require.NoError(t, err)
			assert.Equal(t, want, res.Status, service)
		}
	}

	check(grpchealth.StatusNotServing)
	_, err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Stop()) })
	check(grpchealth.StatusServing)
	require.NoError(t, srv.Stop())
	check(grpchealth.StatusNotServing)
	_, err = srv.Start()
	require.ErrorIs(t, err, http.ErrServerClosed)
}

func TestServerHealthFailedStart(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	srv := New(listener.Addr().String(), false, false)
	_, err = srv.Start()
	require.Error(t, err)
	for _, service := range []string{"", grpchealth.HealthV1ServiceName} {
		res, checkErr := srv.health.Check(t.Context(), &grpchealth.CheckRequest{Service: service})
		require.NoError(t, checkErr)
		assert.Equal(t, grpchealth.StatusNotServing, res.Status)
	}
}

func TestServerReadinessDuringShutdown(t *testing.T) {
	srv := New("127.0.0.1:0", false, false)
	registerHealthTestServices(t, srv, func(context.Context) error {
		require.NoError(t, srv.Stop())
		return nil
	})
	_, err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Stop()) })

	res, err := srv.health.Check(t.Context(), &grpchealth.CheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpchealth.StatusNotServing, res.Status)
}

type healthTestStore struct {
	relayerservice.Store
	ping func(context.Context) error
}

func (s healthTestStore) Ping(ctx context.Context) error { return s.ping(ctx) }

func registerHealthTestServices(t *testing.T, srv *Server, ping func(context.Context) error) {
	t.Helper()
	srv.Register(NewRelayerHandler(relayerservice.New(config.Config{}, healthTestStore{ping: ping}, nil, nil, nil)))
	attestors, err := attestorservice.New(nil)
	require.NoError(t, err)
	srv.Register(NewAttestorHandler(attestors))
}

func startHealthTestServer(t *testing.T, srv *Server) *grpc.ClientConn {
	t.Helper()
	addr, err := srv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, srv.Stop()) })
	conn, err := grpc.NewClient(addr.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	return conn
}
