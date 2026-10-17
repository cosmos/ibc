// SPDX-License-Identifier: Apache-2.0

package ibccli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm/poll"
)

func awaitHealth(ctx context.Context, address string, exited <-chan struct{}) error {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("health client at %s: %w", address, err)
	}
	defer func() { _ = conn.Close() }()
	client := healthpb.NewHealthClient(conn)

	var lastProbeErr error
	err = poll.Until(ctx, 100*time.Millisecond, func(ctx context.Context) (bool, error) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-exited:
			return false, errors.New("process exited before health check passed")
		default:
		}

		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		response, probeErr := client.Check(probeCtx, &healthpb.HealthCheckRequest{Service: ""})
		if probeErr != nil {
			lastProbeErr = probeErr
			return false, nil
		}
		if response.GetStatus() != healthpb.HealthCheckResponse_SERVING {
			lastProbeErr = fmt.Errorf("status is %s", response.GetStatus())
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		if lastProbeErr != nil {
			err = fmt.Errorf("%w (last probe: %w)", err, lastProbeErr)
		}
		return fmt.Errorf("health check at %s: %w", address, err)
	}
	return nil
}
