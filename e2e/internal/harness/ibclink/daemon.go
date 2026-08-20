// SPDX-License-Identifier: Apache-2.0

package ibclink

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"

	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

const (
	// HarnessFinalityOffset treats "latest" minus one block as final on dev chains.
	HarnessFinalityOffset = 1

	defaultStartupTimeout = 60 * time.Second
	// SIGKILL escalation margin after SIGTERM for graceful shutdown.
	stopGrace = 10 * time.Second
	// Backstop: an RPC endpoint that accepts but never responds would hang an unbounded ctx forever.
	statusHTTPTimeout = 30 * time.Second
	killTimeout       = 5 * time.Second

	zeroTxHash = "0x0000000000000000000000000000000000000000000000000000000000000000"
)

type WaitPolicy struct {
	CompletionBudget time.Duration
	StatusPoll       time.Duration
	StabilityWindow  time.Duration
}

// RelayerOptions adapts harness identifiers to the relayer's wire contract.
type RelayerOptions struct {
	// ChainIDs maps harness Chain identifiers to the EVM chain ids (decimal)
	// the relayer is configured with.
	ChainIDs map[string]string
	// ManualRoutes marks route identifiers whose packets must only be relayed
	// on an explicit Relay call.
	ManualRoutes map[string]bool
	// WaitPolicies maps route identifiers to their end-to-end packet wait policy.
	WaitPolicies map[string]WaitPolicy
}

type Relayer struct {
	readiness    relayerv2.ProcessReadiness
	client       relayerv2.RelayerApiServiceClient
	chainIDs     map[string]string
	manualRoutes map[string]bool
	waitPolicies map[string]WaitPolicy
	h            *processHandle
}

func (r *Driver) StartRelayer(ctx context.Context) (*Relayer, error) {
	return startRelayer(ctx, r, r.relayerOpts)
}

type readyResult struct {
	readiness relayerv2.ProcessReadiness
	err       error
}

func startRelayer(ctx context.Context, r *Driver, opts RelayerOptions) (*Relayer, error) {
	processEnv, releaseBinding, err := r.acquireProcessEnv()
	if err != nil {
		return nil, err
	}
	defer func() {
		if releaseBinding != nil {
			releaseBinding()
		}
	}()

	args := append([]string{"relayer", "run"}, r.configArgs()...)
	bin := r.bin
	// Long-lived child: exec.Command (not CommandContext) + Setpgid so Stop can signal the whole group.
	cmd := exec.Command(bin, args...)
	cmd.Env = processEnv
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// First stdout line is the readiness JSON; stderr carries human logs and
	// lands in relayer.log next to the config for post-mortems. A bounded tail
	// is kept in memory because startup failures (config rejections above all)
	// only explain themselves on stderr, and the log file dies with TempDir.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ibc relayer run: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("ibc relayer run: stderr pipe: %w", err)
	}
	logFile, err := os.OpenFile(
		filepath.Join(r.configHome, "relayer.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0o600,
	)
	if err != nil {
		return nil, fmt.Errorf("ibc relayer run: create log: %w", err)
	}
	logs := &logWriter{file: logFile}

	d := &Relayer{
		chainIDs:     opts.ChainIDs,
		manualRoutes: opts.ManualRoutes,
		waitPolicies: opts.WaitPolicies,
	}

	if startErr := cmd.Start(); startErr != nil {
		logs.close()
		return nil, fmt.Errorf("start ibc relayer run (%s): %w", bin, startErr)
	}

	readyCh := make(chan readyResult, 1)
	var drained sync.WaitGroup
	drained.Add(2)
	go func() { defer drained.Done(); drainStdout(stdout, readyCh) }()
	go func() {
		defer drained.Done()
		_, _ = io.Copy(logs, stderr)
		logs.close()
	}()
	d.h = reapProcess(cmd, processHooks{BeforeWait: drained.Wait, AfterWait: releaseBinding})
	releaseBinding = nil

	readiness, err := d.awaitReady(ctx, readyCh)
	if err != nil {
		// kill waits for the stderr drain, so the tail is complete.
		killErr := d.kill()
		return nil, errors.Join(
			fmt.Errorf("%w; logs: %s", err, strings.TrimSpace(string(logs.tailSnapshot()))),
			killErr,
		)
	}
	d.readiness = readiness
	d.client = relayerv2.NewRelayerApiServiceClient(
		&http.Client{Timeout: statusHTTPTimeout},
		"http://"+readiness.HTTP,
		connect.WithGRPC(),
	)
	if err := d.probePacketsEndpoint(ctx); err != nil {
		return nil, errors.Join(err, d.kill())
	}
	return d, nil
}

func drainStdout(rc io.Reader, readyCh chan<- readyResult) {
	br := bufio.NewReader(rc)
	first := true
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			if first {
				first = false
				readyCh <- parseReadiness(line)
			}
		}
		if err != nil {
			if first {
				readyCh <- readyResult{err: fmt.Errorf("no readiness line on stdout: %w", err)}
			}
			return
		}
	}
}

func parseReadiness(line string) readyResult {
	var r relayerv2.ProcessReadiness
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &r); err != nil {
		return readyResult{
			err: fmt.Errorf("first stdout line is not readiness JSON (%q): %w", strings.TrimSpace(line), err),
		}
	}
	if r.Event != relayerv2.ProcessReadinessEvent {
		return readyResult{
			err: fmt.Errorf("invalid readiness: event = %q, want %q", r.Event, relayerv2.ProcessReadinessEvent),
		}
	}
	if r.HTTP == "" {
		return readyResult{err: errors.New("invalid readiness: http is empty")}
	}
	return readyResult{readiness: r}
}

func (d *Relayer) awaitReady(ctx context.Context, readyCh <-chan readyResult) (relayerv2.ProcessReadiness, error) {
	timer := time.NewTimer(defaultStartupTimeout)
	defer timer.Stop()
	select {
	case res := <-readyCh:
		if res.err != nil {
			return relayerv2.ProcessReadiness{}, fmt.Errorf("ibc relayer run: %w", res.err)
		}
		return res.readiness, nil
	case <-timer.C:
		return relayerv2.ProcessReadiness{}, fmt.Errorf("ibc relayer run: not ready within %s", defaultStartupTimeout)
	case <-ctx.Done():
		return relayerv2.ProcessReadiness{}, fmt.Errorf("ibc relayer run: startup canceled: %w", ctx.Err())
	}
}

func (d *Relayer) Ready() relayerv2.ProcessReadiness { return d.readiness }

// ManualRoute reports whether packets on the route are only relayed on an
// explicit Relay call.
func (d *Relayer) ManualRoute(routeID string) bool { return d.manualRoutes[routeID] }

// WaitPolicy returns the configured packet wait policy for routeID.
func (d *Relayer) WaitPolicy(routeID string) (WaitPolicy, bool) {
	policy, ok := d.waitPolicies[routeID]
	return policy, ok
}

// RelayAll submits every configured packet in the source transaction for
// relaying.
func (d *Relayer) RelayAll(ctx context.Context, sourceChainID, sourceTxHash string) error {
	return d.relay(ctx, sourceChainID, &relayerv2.RelayRequest{
		TxHash: sourceTxHash,
		Selection: &relayerv2.RelayRequest_AllPackets{
			AllPackets: &relayerv2.AllPackets{},
		},
	})
}

// RelaySelected submits only packets for the explicit source client and
// sequence selectors.
func (d *Relayer) RelaySelected(
	ctx context.Context,
	sourceChainID string,
	sourceTxHash string,
	packets ...*relayerv2.PacketSelector,
) error {
	return d.relay(ctx, sourceChainID, &relayerv2.RelayRequest{
		TxHash: sourceTxHash,
		Selection: &relayerv2.RelayRequest_SelectedPackets{
			SelectedPackets: &relayerv2.SelectedPackets{Packets: packets},
		},
	})
}

func (d *Relayer) relay(ctx context.Context, sourceChainID string, request *relayerv2.RelayRequest) error {
	chainID, err := d.wireChainID(sourceChainID)
	if err != nil {
		return fmt.Errorf("ibc relay: %w", err)
	}
	request.SourceChainId = chainID
	if _, err := d.client.Relay(ctx, connect.NewRequest(request)); err != nil {
		return fmt.Errorf("ibc relay: %w", err)
	}
	return nil
}

// PacketStatuses returns the relayer's per-packet status for one source
// transaction.
func (d *Relayer) PacketStatuses(
	ctx context.Context,
	sourceChainID string,
	sourceTxHash string,
) ([]*relayerv2.PacketStatus, error) {
	chainID, err := d.wireChainID(sourceChainID)
	if err != nil {
		return nil, fmt.Errorf("ibc packets: %w", err)
	}
	request := &relayerv2.PacketsRequest{
		Filter: &relayerv2.PacketFilter{
			SourceChainId: &chainID,
			SourceTxHash:  &sourceTxHash,
		},
	}

	// A transaction can emit more packets than one page holds. Stopping at the
	// first page would report the rest as absent, which callers read as "not
	// indexed yet" and poll on until their budget expires.
	var statuses []*relayerv2.PacketStatus

	for {
		response, err := d.client.Packets(ctx, connect.NewRequest(request))
		if err != nil {
			return nil, fmt.Errorf("ibc packets: %w", err)
		}

		statuses = append(statuses, response.Msg.GetPackets()...)

		if !response.Msg.GetHasMore() {
			return statuses, nil
		}

		request.Cursor = response.Msg.GetNextCursor()
	}
}

func (d *Relayer) wireChainID(harnessChainID string) (string, error) {
	chainID, ok := d.chainIDs[harnessChainID]
	if !ok {
		return "", fmt.Errorf("no configured chain id for Chain %q", harnessChainID)
	}
	return chainID, nil
}

// probePacketsEndpoint pins the RPC wire contract at startup (not a liveness
// wait): an unknown transaction must succeed and return nothing. Drivers
// without a chain mapping have nothing to probe.
func (d *Relayer) probePacketsEndpoint(ctx context.Context) error {
	for _, chainID := range d.chainIDs {
		response, err := d.client.Packets(ctx, connect.NewRequest(&relayerv2.PacketsRequest{
			Filter: &relayerv2.PacketFilter{
				SourceChainId: &chainID,
				SourceTxHash:  &[]string{zeroTxHash}[0],
			},
		}))
		if err != nil {
			return fmt.Errorf("ibc relayer run: packets probe: %w", err)
		}
		if len(response.Msg.GetPackets()) != 0 {
			return fmt.Errorf("ibc relayer run: packets probe: unknown transaction returned packets")
		}
		return nil
	}
	return nil
}

func (d *Relayer) Stop(ctx context.Context) error {
	return d.h.signalAndWait(ctx, syscall.SIGTERM, stopGrace)
}

func (d *Relayer) kill() error {
	ctx, cancel := context.WithTimeout(context.Background(), killTimeout)
	defer cancel()
	return d.h.signalAndWait(ctx, syscall.SIGKILL, 0)
}
