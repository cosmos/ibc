package ibclink

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/internal/proc"
)

const (
	defaultStartupTimeout = 60 * time.Second
	// SIGKILL escalation margin after SIGTERM for graceful HTTP drain.
	stopGrace = 10 * time.Second
	// Backstop: a status endpoint that accepts but never responds would hang an unbounded ctx forever.
	statusHTTPTimeout = 30 * time.Second
	// Best-effort pre-stop snapshot; must not delay teardown on a wedged daemon.
	finalStatusTimeout = 2 * time.Second
)

// Per-instance numbering so a Restart's os.Create does not truncate the prior daemon log.
var daemonSeq atomic.Int64

type Daemon interface {
	Ready() wire.Readiness
	Status(ctx context.Context, q wire.StatusQuery) (*wire.Status, error)
	Relay(ctx context.Context, req wire.RelayRequest) (*wire.RelayResult, error)
	Stop(ctx context.Context) error
	Restart(ctx context.Context) (Daemon, error)
	Logs() io.Reader
	FinalStatus() (*wire.Status, bool)
	LogLabel() string
}

type daemon struct {
	runner    *runner
	readiness wire.Readiness
	httpAddr  string
	http      *http.Client

	logSeq int

	out *safeWriter
	h   *proc.Handle

	mu          sync.Mutex
	finalStatus *wire.Status
}

var _ Daemon = (*daemon)(nil)

func (r *runner) Run(ctx context.Context) (Daemon, error) {
	return startDaemon(ctx, r)
}

type readyResult struct {
	readiness wire.Readiness
	err       error
}

func startDaemon(ctx context.Context, r *runner) (*daemon, error) {
	const label = "relayer run"
	args := append([]string{"relayer", "run"}, r.configArgs()...)
	bin, err := r.binFor(label)
	if err != nil {
		return nil, err
	}
	// Long-lived child: exec.Command (not CommandContext) + Setpgid so Stop can signal the whole group.
	cmd := exec.Command(bin, args...)
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

	seq := int(daemonSeq.Add(1))
	out := &safeWriter{}
	if r.logPath != "" {
		if f, ferr := os.Create(daemonLogPath(r.logPath, seq)); ferr == nil {
			out.f = f
		}
	}

	d := &daemon{
		runner: r,
		logSeq: seq,
		http:   &http.Client{Timeout: statusHTTPTimeout},
		out:    out,
	}

	if startErr := cmd.Start(); startErr != nil {
		out.close()
		return nil, fmt.Errorf("start ibc relayer run (%s): %w", bin, startErr)
	}
	if r.logPath != "" {
		pid := strconv.Itoa(cmd.Process.Pid)
		_ = os.WriteFile(daemonPidPath(r.logPath, seq), []byte(pid+"\n"), 0o644)
	}

	readyCh := make(chan readyResult, 1)
	var drained sync.WaitGroup
	drained.Add(2)
	go func() { defer drained.Done(); d.drainStdout(stdout, readyCh) }()
	go func() { defer drained.Done(); d.drainStderr(stderr) }()
	d.h = proc.Reap(cmd, drained.Wait)
	go func() {
		<-d.h.Done()
		out.close()
	}()

	readiness, err := d.awaitReady(ctx, readyCh)
	if err != nil {
		_ = d.kill()
		return nil, err
	}
	d.readiness = readiness
	d.httpAddr = readiness.Status.HTTP
	if err := d.probeHealth(ctx); err != nil {
		_ = d.kill()
		return nil, err
	}
	return d, nil
}

func (d *daemon) drainStdout(rc io.Reader, readyCh chan<- readyResult) {
	br := bufio.NewReader(rc)
	first := true
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			d.out.writeLine(line)
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
	var r wire.Readiness
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

func (d *daemon) drainStderr(rc io.Reader) {
	br := bufio.NewReader(rc)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			d.out.writeLine(line)
		}
		if err != nil {
			return
		}
	}
}

func (d *daemon) awaitReady(ctx context.Context, readyCh <-chan readyResult) (wire.Readiness, error) {
	timer := time.NewTimer(defaultStartupTimeout)
	defer timer.Stop()
	select {
	case res := <-readyCh:
		if res.err != nil {
			return wire.Readiness{}, fmt.Errorf("ibc relayer run: %w", res.err)
		}
		return res.readiness, nil
	case <-timer.C:
		return wire.Readiness{}, fmt.Errorf("ibc relayer run: not ready within %s", defaultStartupTimeout)
	case <-ctx.Done():
		return wire.Readiness{}, fmt.Errorf("ibc relayer run: startup canceled: %w", ctx.Err())
	}
}

func (d *daemon) Ready() wire.Readiness { return d.readiness }

func (d *daemon) Logs() io.Reader { return bytes.NewReader(d.out.snapshot()) }

func (d *daemon) LogLabel() string { return fmt.Sprintf("ibc-daemon-%d", d.logSeq) }

func (d *daemon) Status(ctx context.Context, q wire.StatusQuery) (*wire.Status, error) {
	u := d.apiURL(wire.StatusPath)
	vals := url.Values{}
	if q.RouteID != "" {
		vals.Set(wire.StatusQueryRoute, q.RouteID)
	}
	if q.PacketID != "" {
		vals.Set(wire.StatusQueryPacket, q.PacketID)
	}
	u.RawQuery = vals.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("ibc status: build request: %w", err)
	}
	var status wire.Status
	if err := d.doJSON(req, "ibc status", &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (d *daemon) Relay(ctx context.Context, in wire.RelayRequest) (*wire.RelayResult, error) {
	u := d.apiURL(wire.RelayPath)
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("ibc relay: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ibc relay: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	var result wire.RelayResult
	if err := d.doJSON(req, "ibc relay", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Pins the /health wire contract at startup (not a liveness wait).
func (d *daemon) probeHealth(ctx context.Context) error {
	u := d.apiURL(wire.HealthPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("ibc relayer run: health: build request: %w", err)
	}
	return d.doJSON(req, "ibc relayer run: health", nil)
}

func (d *daemon) apiURL(path string) url.URL {
	return url.URL{Scheme: "http", Host: d.httpAddr, Path: path}
}

func (d *daemon) doJSON(req *http.Request, label string, out any) error {
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %s %s: %w", label, req.Method, req.URL.String(), err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf(
			"%s: %s %s -> %s: %s",
			label,
			req.Method,
			req.URL.String(),
			resp.Status,
			strings.TrimSpace(string(body)),
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

func (d *daemon) Stop(ctx context.Context) error {
	d.snapshotFinalStatus(ctx)
	return d.h.SignalAndWait(ctx, syscall.SIGTERM, stopGrace)
}

func (d *daemon) snapshotFinalStatus(ctx context.Context) {
	if d.httpAddr == "" {
		return
	}
	sctx, cancel := context.WithTimeout(ctx, finalStatusTimeout)
	defer cancel()
	s, err := d.Status(sctx, wire.StatusQuery{})
	if err != nil {
		return
	}
	d.mu.Lock()
	d.finalStatus = s
	d.mu.Unlock()
}

func (d *daemon) FinalStatus() (*wire.Status, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.finalStatus, d.finalStatus != nil
}

func (d *daemon) kill() error {
	return d.h.SignalAndWait(context.Background(), syscall.SIGKILL, 0)
}

func (d *daemon) Restart(ctx context.Context) (Daemon, error) {
	if err := d.Stop(ctx); err != nil {
		return nil, fmt.Errorf("restart: stop current daemon: %w", err)
	}
	nd, err := startDaemon(ctx, d.runner)
	if err != nil {
		return nil, fmt.Errorf("restart: start new daemon: %w", err)
	}
	return nd, nil
}

func daemonLogPath(runnerLog string, instance int) string {
	return fmt.Sprintf("%s-daemon-%d.log", strings.TrimSuffix(runnerLog, ".log"), instance)
}

func daemonPidPath(runnerLog string, instance int) string {
	return fmt.Sprintf("%s-daemon-%d.pid", strings.TrimSuffix(runnerLog, ".log"), instance)
}

type safeWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
	f   *os.File
}

func (w *safeWriter) writeLine(s string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.WriteString(s)
	if w.f != nil {
		_, _ = w.f.WriteString(s)
	}
}

func (w *safeWriter) snapshot() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buf.Bytes()...)
}

func (w *safeWriter) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f != nil {
		_ = w.f.Close()
		w.f = nil
	}
}
