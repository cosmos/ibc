// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type testHealthService struct {
	healthpb.UnimplementedHealthServer
	check func(context.Context, *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error)
}

func (s testHealthService) Check(
	ctx context.Context,
	request *healthpb.HealthCheckRequest,
) (*healthpb.HealthCheckResponse, error) {
	return s.check(ctx, request)
}

func startTestHealthServer(t *testing.T, service testHealthService) string {
	t.Helper()
	listener, err := net.Listen("tcp", loopbackAnyPort)
	require.NoError(t, err)
	server := grpc.NewServer()
	healthpb.RegisterHealthServer(server, service)
	t.Cleanup(server.Stop)
	go func() { _ = server.Serve(listener) }()
	return listener.Addr().String()
}

func TestAwaitHealthRetriesUntilServing(t *testing.T) {
	var calls atomic.Int32
	address := startTestHealthServer(t, testHealthService{check: func(
		_ context.Context, request *healthpb.HealthCheckRequest,
	) (*healthpb.HealthCheckResponse, error) {
		if request.GetService() != "" {
			return nil, status.Error(codes.InvalidArgument, "expected overall readiness")
		}
		switch calls.Add(1) {
		case 1:
			return nil, status.Error(codes.Unavailable, "starting")
		case 2:
			return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_NOT_SERVING}, nil
		case 3:
			return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_UNKNOWN}, nil
		case 4:
			return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVICE_UNKNOWN}, nil
		default:
			return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
		}
	}})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, awaitHealth(ctx, address, nil))
	require.Equal(t, int32(5), calls.Load())
}

func TestAwaitHealthReportsDeadlineAndLastStatus(t *testing.T) {
	address := startTestHealthServer(t, testHealthService{check: func(
		context.Context, *healthpb.HealthCheckRequest,
	) (*healthpb.HealthCheckResponse, error) {
		return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_NOT_SERVING}, nil
	}})
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	err := awaitHealth(ctx, address, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, address)
	require.ErrorContains(t, err, "NOT_SERVING")
}

func TestAwaitHealthPreservesLastStatusAfterRPCError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	address := startTestHealthServer(t, testHealthService{check: func(
		probeCtx context.Context, _ *healthpb.HealthCheckRequest,
	) (*healthpb.HealthCheckResponse, error) {
		switch calls.Add(1) {
		case 1:
			return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_NOT_SERVING}, nil
		case 2:
			return nil, status.Error(codes.DeadlineExceeded, "probe timed out")
		}
		cancel()
		<-probeCtx.Done()
		return nil, probeCtx.Err()
	}})
	err := awaitHealth(ctx, address, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorContains(t, err, "NOT_SERVING")
	require.Equal(t, int32(3), calls.Load())
}

func TestAwaitHealthCancelsBlockedRPC(t *testing.T) {
	started := make(chan struct{})
	address := startTestHealthServer(t, testHealthService{check: func(
		ctx context.Context, _ *healthpb.HealthCheckRequest,
	) (*healthpb.HealthCheckResponse, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- awaitHealth(ctx, address, nil) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("health RPC did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("health RPC did not stop after cancellation")
	}
}

func TestAwaitHealthStopsWhenProcessExits(t *testing.T) {
	exited := make(chan struct{})
	address := startTestHealthServer(t, testHealthService{check: func(
		context.Context, *healthpb.HealthCheckRequest,
	) (*healthpb.HealthCheckResponse, error) {
		close(exited)
		return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_NOT_SERVING}, nil
	}})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err := awaitHealth(ctx, address, exited)
	require.ErrorContains(t, err, "process exited")
	require.ErrorContains(t, err, "NOT_SERVING")
}
