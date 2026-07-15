package ibclink

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

const (
	defaultStartupTimeout = 60 * time.Second
	// SIGKILL escalation margin after SIGTERM for graceful HTTP drain.
	stopGrace = 10 * time.Second
	// Backstop: a status endpoint that accepts but never responds would hang an unbounded ctx forever.
	statusHTTPTimeout = 30 * time.Second
	killTimeout       = 5 * time.Second
	errorBodyLimit    = 512
)

type Relayer struct {
	readiness relayercmd.Readiness
	httpAddr  string
	http      *http.Client
	h         *processHandle
}

func (r *Driver) StartRelayer(ctx context.Context) (*Relayer, error) {
	return startRelayer(ctx, r)
}

type readyResult struct {
	readiness relayercmd.Readiness
	err       error
}

func startRelayer(ctx context.Context, r *Driver) (*Relayer, error) {
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
	cmd.Env = processEnv.variables
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// First stdout line is the readiness JSON; stderr carries human logs and must stay separate.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ibc relayer run: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("ibc relayer run: stderr pipe: %w", err)
	}

	d := &Relayer{http: &http.Client{Timeout: statusHTTPTimeout}}

	if startErr := cmd.Start(); startErr != nil {
		return nil, fmt.Errorf("start ibc relayer run (%s): %w", bin, startErr)
	}

	readyCh := make(chan readyResult, 1)
	var drained sync.WaitGroup
	drained.Add(2)
	go func() { defer drained.Done(); drainStdout(stdout, readyCh) }()
	go func() { defer drained.Done(); _, _ = io.Copy(io.Discard, stderr) }()
	d.h = reapProcess(cmd, processHooks{BeforeWait: drained.Wait, AfterWait: releaseBinding})
	releaseBinding = nil

	readiness, err := d.awaitReady(ctx, readyCh)
	if err != nil {
		return nil, errors.Join(err, d.kill())
	}
	d.readiness = readiness
	d.httpAddr = readiness.Status.HTTP
	if err := d.probeHealth(ctx); err != nil {
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
	var r relayercmd.Readiness
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &r); err != nil {
		return readyResult{
			err: fmt.Errorf("first stdout line is not readiness JSON (%q): %w", strings.TrimSpace(line), err),
		}
	}
	if err := r.Validate(); err != nil {
		return readyResult{err: fmt.Errorf("invalid readiness: %w", err)}
	}
	return readyResult{readiness: r}
}

func (d *Relayer) awaitReady(ctx context.Context, readyCh <-chan readyResult) (relayercmd.Readiness, error) {
	timer := time.NewTimer(defaultStartupTimeout)
	defer timer.Stop()
	select {
	case res := <-readyCh:
		if res.err != nil {
			return relayercmd.Readiness{}, fmt.Errorf("ibc relayer run: %w", res.err)
		}
		return res.readiness, nil
	case <-timer.C:
		return relayercmd.Readiness{}, fmt.Errorf("ibc relayer run: not ready within %s", defaultStartupTimeout)
	case <-ctx.Done():
		return relayercmd.Readiness{}, fmt.Errorf("ibc relayer run: startup canceled: %w", ctx.Err())
	}
}

func (d *Relayer) Ready() relayercmd.Readiness { return d.readiness }

func (d *Relayer) Status(ctx context.Context, q relayercmd.StatusQuery) (*relayercmd.Status, error) {
	u := d.apiURL(relayercmd.StatusPath)
	vals := url.Values{}
	if q.RouteID != "" {
		vals.Set(relayercmd.StatusQueryRoute, q.RouteID)
	}
	if q.PacketID != "" {
		vals.Set(relayercmd.StatusQueryPacket, q.PacketID)
	}
	u.RawQuery = vals.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("ibc status: build request: %w", err)
	}
	var status relayercmd.Status
	if err := d.doJSON(req, "ibc status", &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (d *Relayer) Relay(ctx context.Context, in relayercmd.RelayRequest) (*relayercmd.RelayResult, error) {
	u := d.apiURL(relayercmd.RelayPath)
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("ibc relay: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ibc relay: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	var result relayercmd.RelayResult
	if err := d.doJSON(req, "ibc relay", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Pins the /health wire contract at startup (not a liveness wait).
func (d *Relayer) probeHealth(ctx context.Context) error {
	u := d.apiURL(relayercmd.HealthPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("ibc relayer run: health: build request: %w", err)
	}
	return d.doJSON(req, "ibc relayer run: health", nil)
}

func (d *Relayer) apiURL(path string) url.URL {
	return url.URL{Scheme: "http", Host: d.httpAddr, Path: path}
}

func (d *Relayer) doJSON(req *http.Request, label string, out any) error {
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %s %s: %w", label, req.Method, req.URL.String(), err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit+1))
		if readErr != nil {
			return fmt.Errorf(
				"%s: %s %s -> %s: read response body: %w",
				label,
				req.Method,
				req.URL.String(),
				resp.Status,
				readErr,
			)
		}
		bodyText := "<response body omitted: exceeds limit>"
		if len(body) <= errorBodyLimit {
			bodyText = strings.TrimSpace(string(body))
		}
		return fmt.Errorf(
			"%s: %s %s -> %s: %s",
			label,
			req.Method,
			req.URL.String(),
			resp.Status,
			bodyText,
		)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s: decode response: %w", label, err)
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
