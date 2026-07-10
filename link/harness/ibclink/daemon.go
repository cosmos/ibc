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
	// defaultStartupTimeout bounds how long Run waits for the daemon's readiness line. The daemon dials
	// every configured chain (each up to its own startup-dial budget) and binds the status server before
	// it can announce readiness, so the bound is generous; readiness is a semantic signal (the parsed
	// readiness JSON), never a sleep.
	defaultStartupTimeout = 60 * time.Second
	// stopGrace is how long Stop waits after SIGTERM for the daemon's graceful HTTP drain before it
	// escalates to SIGKILL. The relay drains within a few seconds; this leaves margin.
	stopGrace = 10 * time.Second
	// statusHTTPTimeout is the hard ceiling on a single status GET. The status endpoint is a local,
	// in-memory query that answers in well under a second, so this is only a backstop: without it a status
	// endpoint that accepts the connection but never responds would hang a caller that passed an unbounded
	// context forever.
	statusHTTPTimeout = 30 * time.Second
	// finalStatusTimeout bounds the last status snapshot Stop takes before signaling the process. It is
	// deliberately tight: the snapshot is best-effort diagnostics, and a wedged daemon (the usual reason
	// Stop is being called on a failed run) must not delay its own teardown.
	finalStatusTimeout = 2 * time.Second
)

// daemonSeq assigns each started daemon a process-unique instance number. A Restart spawns a fresh
// process against the same runner (so the same log-path base), so the on-disk daemon log is namespaced
// per instance — otherwise the second instance's os.Create would truncate the first instance's log and
// lose it (the restart-mid-packet path).
var daemonSeq atomic.Int64

// Daemon drives a running `ibc relayer run` instance as a black box: it never reaches into the
// relayer's internals, only reads the readiness line it printed, hits its status HTTP API, execs the
// one-shot app-action command against the same shared config/DB, and signals the process to stop. The
// concrete *daemon is returned by Runner.Run.
type Daemon interface {
	// Ready returns the readiness JSON the daemon printed as its first stdout line (captured at Run).
	Ready() wire.Readiness
	// Status GETs the daemon's status HTTP endpoint (advertised in the readiness line), optionally
	// filtered. It is the harness's cross-check surface — the better signal for pending packets.
	Status(ctx context.Context, q wire.StatusQuery) (*wire.Status, error)
	// Relay POSTs a manual relay request to the daemon's relay HTTP endpoint.
	Relay(ctx context.Context, req wire.RelayRequest) (*wire.RelayResult, error)
	// Stop takes a final bounded status snapshot (see FinalStatus), then sends SIGTERM and waits for a
	// graceful exit (escalating to SIGKILL past the grace window).
	Stop(ctx context.Context) error
	// Restart stops this daemon and starts a fresh one against the same config, returning the new
	// Daemon. The stub resumes any pending packets from sqlite, so in-flight work completes after.
	Restart(ctx context.Context) (Daemon, error)
	// Logs returns a snapshot reader over the daemon's captured combined output, for
	// diagnostics.
	Logs() io.Reader
	// FinalStatus returns the status snapshot Stop captured just before signaling the process, if one was
	// obtained. It makes diagnostics capture time-independent: a bundle assembled after the process is
	// gone still carries the daemon's last self-report, so no caller has to race a capture against Stop.
	FinalStatus() (*wire.Status, bool)
	// LogLabel is the per-instance key a diagnostics bundle should file this daemon's log under. It
	// shares its instance number with the on-disk daemon log path, so a captured bundle entry and the
	// preserved KEEP_AFTER_TEST file always carry the same number (a Restart spawns a new instance with
	// a new number, so the two never overwrite each other in a name-keyed bundle).
	LogLabel() string
}

// daemon is the concrete Daemon: the supervised relayer subprocess plus the captured readiness/status
// wiring. It holds the runner it was started from so Restart can spawn a fresh instance against the
// same config.
type daemon struct {
	runner    *runner
	readiness wire.Readiness
	httpAddr  string // status HTTP host:port from the readiness line
	http      *http.Client

	logSeq int // process-unique instance number; shared by the on-disk log path and LogLabel

	out *safeWriter  // captured combined output, for Logs() + the daemon run log
	h   *proc.Handle // owns cmd.Wait() and the stop/reap state machine

	mu          sync.Mutex
	finalStatus *wire.Status // written under mu by snapshotFinalStatus (successful probes only); read via FinalStatus
}

var _ Daemon = (*daemon)(nil)

// Run starts the relayer daemon and blocks on its semantic readiness.
func (r *runner) Run(ctx context.Context) (Daemon, error) {
	return startDaemon(ctx, r)
}

// readyResult carries the outcome of reading the first stdout line: either the parsed readiness or the
// reason it could not be obtained (bad line, or the process produced no stdout before exiting).
type readyResult struct {
	readiness wire.Readiness
	err       error
}

// startDaemon launches `ibc relayer run` as a long-lived child (not via exec.CommandContext with
// the one-shot timeout — its lifetime is owned by Stop/kill), reads + parses the first stdout line as
// the readiness signal, and keeps draining both streams into a capture buffer + log file.
func startDaemon(ctx context.Context, r *runner) (*daemon, error) {
	const label = "relayer run"
	args := append([]string{"relayer", "run"}, r.configArgs()...)
	bin, err := r.binFor(label)
	if err != nil {
		return nil, err
	}
	// Deliberately exec.Command, not exec.CommandContext: a per-command deadline must never apply to the
	// long-lived daemon. Setpgid lets Stop/kill signal the whole subtree.
	cmd := exec.Command(bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Keep stdout and stderr separate: the readiness JSON is the first stdout line, while human logs go
	// to stderr. Merging them would make "first line" non-deterministic across the two streams.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ibc relayer run: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("ibc relayer run: stderr pipe: %w", err)
	}

	// Assign the instance number once, up front, so it is stable whether or not a log file is configured
	// and both the on-disk path and LogLabel derive from the same value.
	seq := int(daemonSeq.Add(1))
	out := &safeWriter{}
	if r.logPath != "" {
		// Best-effort: a dedicated daemon log next to the one-shot run log, so KEEP_AFTER_TEST preserves
		// the daemon's output without interleaving it with the per-command appends to logPath.
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
		// Best-effort pid file beside the daemon log. The daemon is its own process group (Setpgid), so a
		// KeepOnClose run that outlives the test binary leaves it running; the pid file is what lets reset
		// tooling sweep host daemons the way labels let it sweep containers.
		pid := strconv.Itoa(cmd.Process.Pid)
		_ = os.WriteFile(daemonPidPath(r.logPath, seq), []byte(pid+"\n"), 0o644)
	}

	// Drain both streams to EOF (which arrives when the process exits). cmd.Wait must not run until both
	// drains finish, so the WaitGroup's Wait is the handle's preWait barrier. The stdout drainer also
	// extracts the readiness line, and the log sink closes once the process is reaped.
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
		// Tear down the half-started daemon so a failed Run never leaks a relayer process.
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

// drainStdout reads the daemon's stdout line by line. The first line is parsed as the readiness JSON
// and signaled on readyCh; every line is captured verbatim (stdout is machine JSON we must not
// mangle). It runs to EOF so cmd.Wait stays safe.
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
				// No stdout line at all before the stream closed — the daemon almost certainly exited
				// before announcing readiness.
				readyCh <- readyResult{err: fmt.Errorf("no readiness line on stdout: %w", err)}
			}
			return
		}
	}
}

// parseReadiness decodes a stdout line as the readiness event, validating the discriminant field.
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

// drainStderr captures the daemon's stderr to EOF.
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

// awaitReady blocks until the readiness line arrives, the startup budget elapses, or ctx is canceled.
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

// Status GETs the daemon's status HTTP endpoint.
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

// Relay POSTs a manual relay request to the daemon.
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

// probeHealth pins the /health wire contract at startup. Readiness already implies the API is
// serving, so this is not a liveness wait: it fails a start whose daemon does not serve the health
// endpoint, keeping /health a tested contract in every lane.
func (d *daemon) probeHealth(ctx context.Context) error {
	u := d.apiURL(wire.HealthPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("ibc relayer run: health: build request: %w", err)
	}
	return d.doJSON(req, "ibc relayer run: health", nil)
}

// apiURL addresses one daemon API endpoint: a plain-HTTP URL on the daemon's local status listener.
func (d *daemon) apiURL(path string) url.URL {
	return url.URL{Scheme: "http", Host: d.httpAddr, Path: path}
}

// doJSON is the shared tail of every daemon API call: execute the request, treat any non-200 as an
// error carrying the first 512 bytes of the response body, and decode the JSON response into out
// (skipped when out is nil). label prefixes every error so failures stay attributable per verb.
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

// Stop takes the final status snapshot, then signals SIGTERM and waits for the daemon's graceful exit,
// escalating to SIGKILL past the grace (the stop/reap state machine lives in proc.Handle).
func (d *daemon) Stop(ctx context.Context) error {
	d.snapshotFinalStatus(ctx)
	return d.h.SignalAndWait(ctx, syscall.SIGTERM, stopGrace)
}

// snapshotFinalStatus captures the daemon's status-API view before the process is signaled, so
// FinalStatus can serve it after the process is gone. Best-effort and tightly bounded; it stores only
// successful probes, so a repeat Stop against the dead endpoint (e.g. via Restart) fails its probe
// fast and leaves the earlier snapshot intact.
func (d *daemon) snapshotFinalStatus(ctx context.Context) {
	if d.httpAddr == "" {
		return // never became ready; there is no status endpoint to snapshot
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

// kill signals SIGKILL and waits for the process to be reaped.
func (d *daemon) kill() error {
	return d.h.SignalAndWait(context.Background(), syscall.SIGKILL, 0)
}

// Restart stops this daemon and starts a fresh one against the same config. The stub resumes pending
// packets from sqlite on start, so work in flight at stop time completes on the new instance.
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

// daemonLogPath derives a per-instance daemon log path from the runner's one-shot run-log path, suffixing
// the instance number (see daemonSeq).
func daemonLogPath(runnerLog string, instance int) string {
	return fmt.Sprintf("%s-daemon-%d.log", strings.TrimSuffix(runnerLog, ".log"), instance)
}

// daemonPidPath is daemonLogPath's sibling pid file for the same instance.
func daemonPidPath(runnerLog string, instance int) string {
	return fmt.Sprintf("%s-daemon-%d.pid", strings.TrimSuffix(runnerLog, ".log"), instance)
}

// safeWriter captures the daemon's combined output for both Logs() (an in-memory buffer) and an
// optional on-disk log, serialized so the two drain goroutines (stdout + stderr) can write
// concurrently.
type safeWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
	f   *os.File // nil if no log file was configured
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
