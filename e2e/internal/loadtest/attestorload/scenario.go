// SPDX-License-Identifier: Apache-2.0

// Package attestorload is the attestor load scenario: fixed-rate packet and
// state attestation requests against the gRPC API of the Attestor that
// observes Chain A. No relayer runs.
package attestorload

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"math/rand/v2"
	"net/http"
	"slices"
	"time"

	"connectrpc.com/connect"

	attestorv2 "github.com/cosmos/ibc/cli/api/v2/attestor"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/traffic"
	"github.com/cosmos/ibc/e2e/internal/loadtest/session"
	"github.com/cosmos/ibc/e2e/internal/loadtest/simpleload"
)

const (
	// seedPackets is the number of packets the packet phase attests at
	// random, all sent in one transaction.
	seedPackets  = 10
	seedAmount   = 100_000
	PhasePackets = "packets"
	PhaseState   = "state"
)

// Config shapes one attestor scenario run. The CLI validates it.
type Config struct {
	Packets simpleload.Spec
	State   simpleload.Spec
	// Phases are PhasePackets and PhaseState runs, in order.
	Phases []string
	// MaxErrorRate is the largest share of sent calls that may fail or time
	// out in a phase that passes.
	MaxErrorRate float64
}

// PhaseReport is the outcome of one phase.
type PhaseReport struct {
	Phase string `json:"phase"`
	simpleload.Result
	Passed bool `json:"passed"`
}

// Report is the outcome of the attestor scenario.
type Report struct {
	Scenario     string         `json:"scenario"`
	Provider     string         `json:"provider"`
	Attestor     string         `json:"attestor"`
	MaxErrorRate float64        `json:"max_error_rate"`
	Passed       bool           `json:"passed"`
	Error        string         `json:"error,omitempty"`
	Phases       []*PhaseReport `json:"phases"`
}

// SessionOptions adds what cfg needs to base: the A-to-B route's applications
// when the packets phase seeds packets, and no relayer.
func SessionOptions(base session.Options, cfg Config) session.Options {
	base.Routes = nil
	if slices.Contains(cfg.Phases, PhasePackets) {
		base.Routes = []traffic.Route{traffic.AtoB(traffic.ChainA, traffic.ChainB)}
	}
	base.StartRelayer = false
	return base
}

type attestablePacket struct {
	height  uint64
	encoded []byte
}

// Run runs the phases in order and returns the report, with an error unless
// every phase passed judge. A phase that cannot start, or an interruption,
// ends the run; the report covers every phase that sent calls, the
// interrupted one included, which fails.
func Run(ctx context.Context, s *session.Session, cfg Config) (*Report, error) {
	report := &Report{Scenario: "attestor", Provider: string(s.Provider), MaxErrorRate: cfg.MaxErrorRate}
	err := run(ctx, s, cfg, report)
	report.Passed = err == nil
	if err != nil {
		report.Error = err.Error()
	}
	return report, err
}

func run(ctx context.Context, s *session.Session, cfg Config, report *Report) error {
	// The Client on chain B tracks chain A, so its Attestor observes chain A.
	attestor, err := s.Env.Attestor(traffic.MeshAttestorFor(traffic.ChainB, traffic.ChainA))
	if err != nil {
		return err
	}
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	transport := &http.Transport{Protocols: protocols}
	defer transport.CloseIdleConnections()
	rpc := attestorv2.NewAttestationServiceClient(
		&http.Client{Transport: transport},
		"http://"+attestor.Endpoint(),
		connect.WithGRPC(),
	)
	name := string(attestor.ID())
	report.Attestor = name

	var errs []error
	for _, phase := range cfg.Phases {
		result, phaseErr := runPhase(ctx, s, attestor, rpc, name, phase, cfg)
		if result == nil {
			errs = append(errs, phaseErr)
			break
		}
		failure := phaseErr
		if failure == nil {
			failure = judge(phase, result, cfg.MaxErrorRate)
		}
		report.Phases = append(report.Phases, &PhaseReport{Phase: phase, Result: *result, Passed: failure == nil})
		if failure != nil {
			errs = append(errs, failure)
		}
		if phaseErr != nil {
			break
		}
	}
	return errors.Join(errs...)
}

// judge fails a phase unless at least one call succeeded and failed plus
// timed-out calls stayed within maxErrorRate of those sent.
func judge(phase string, result *simpleload.Result, maxErrorRate float64) error {
	errored := result.Failed + result.TimedOut
	if result.Succeeded > 0 && float64(errored) <= maxErrorRate*float64(result.Sent) {
		return nil
	}
	return fmt.Errorf("%s: %d succeeded, %d of %d sent failed or timed out (max error rate %g)",
		phase, result.Succeeded, errored, result.Sent, maxErrorRate)
}

// runPhase prepares phase and sends its load. Its result is nil if the phase
// could not start; it fails, with the calls sent so far, if ctx ends.
func runPhase(
	ctx context.Context,
	s *session.Session,
	attestor *environment.Attestor,
	rpc attestorv2.AttestationServiceClient,
	name, phase string,
	cfg Config,
) (*simpleload.Result, error) {
	var spec simpleload.Spec
	var call simpleload.Call
	switch phase {
	case PhasePackets:
		packets, err := seed(ctx, s, attestor)
		if err != nil {
			return nil, err
		}
		spec, call = cfg.Packets, packetCall(rpc, name, packets)
	case PhaseState:
		latest, err := attestor.LatestHeight(ctx)
		if err != nil {
			return nil, fmt.Errorf("attestor latest height: %w", err)
		}
		if latest == 0 {
			return nil, errors.New("attestor reports no attestable height")
		}
		spec, call = cfg.State, stateCall(rpc, name, latest)
	default:
		return nil, fmt.Errorf("unknown phase %q", phase)
	}
	slog.InfoContext(ctx, "attestor load phase", "phase", phase, "duration", spec.Duration,
		"rps", spec.RatePerSecond)
	result := simpleload.SimpleLoad(ctx, spec, call)
	if err := ctx.Err(); err != nil {
		return &result, fmt.Errorf("%s: interrupted after %s: %w", phase,
			time.Duration(result.DurationMS)*time.Millisecond, err)
	}
	return &result, nil
}

// WriteText writes the report for people.
func (r *Report) WriteText(w io.Writer) error {
	var out bytes.Buffer
	verdict := "PASS"
	if !r.Passed {
		verdict = "FAIL"
	}
	fmt.Fprintf(&out, "\nAttestor load: %s (%s, attestor %s, max error rate %g)\n",
		verdict, r.Provider, r.Attestor, r.MaxErrorRate)
	for _, p := range r.Phases {
		fmt.Fprintf(&out, "\n[%s] %s: %.1f of %d rps sent\n", p.Phase,
			time.Duration(p.DurationMS)*time.Millisecond, p.AchievedRPS, p.RatePerSecond)
		fmt.Fprintf(&out, "  calls:   sent=%d succeeded=%d failed=%d timed_out=%d\n",
			p.Sent, p.Succeeded, p.Failed, p.TimedOut)
		fmt.Fprintf(&out, "  latency: %s\n", p.Latency)
		if p.FirstError != "" {
			fmt.Fprintf(&out, "  first error: %s\n", p.FirstError)
		}
	}
	if r.Error != "" {
		fmt.Fprintf(&out, "\nerrors:\n%s\n", r.Error)
	}
	_, err := w.Write(out.Bytes())
	return err
}

// seed sends seedPackets IFT transfers from chain A in one batch transaction
// and waits until the attestor can attest the block that included it. The
// packets are never relayed, so their commitments stay in place.
func seed(ctx context.Context, s *session.Session, attestor *environment.Attestor) ([]attestablePacket, error) {
	route := traffic.AtoB(traffic.ChainA, traffic.ChainB)
	app, err := traffic.NewIFT(s.Env, s.Deployment, s.Deployer, route)
	if err != nil {
		return nil, err
	}
	requests := make([]traffic.IFTRequest, seedPackets)
	for i := range requests {
		requests[i] = traffic.IFTRequest{Amount: big.NewInt(seedAmount)}
	}
	batch, err := app.SendBatch(ctx, requests)
	if err != nil {
		return nil, fmt.Errorf("seed transfers: %w", err)
	}
	source, ok := s.Deployment.Chain(route.Source)
	if !ok {
		return nil, fmt.Errorf("no deployment on Chain %q", route.Source)
	}
	// Each SendPacket log's data is its packet's encoding as attested.
	sent, err := traffic.SentPackets(batch.Receipt(), source.ICS26Router)
	if err != nil {
		return nil, err
	}
	height := batch.Receipt().BlockNumber.Uint64()
	packets := make([]attestablePacket, len(sent))
	for i, event := range sent {
		packets[i] = attestablePacket{height: height, encoded: event.Raw.Data}
	}
	if err := awaitAttestable(ctx, s, attestor, route.Source, height); err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "seeded packets", "count", len(packets), "height", height)
	return packets, nil
}

// awaitAttestable waits within the Chain's completion budget until the
// attestor's latest height reaches height.
func awaitAttestable(
	ctx context.Context,
	s *session.Session,
	attestor *environment.Attestor,
	chainID environment.ChainID,
	height uint64,
) error {
	chain, err := s.Env.Chain(chainID)
	if err != nil {
		return err
	}
	timing := chain.Timing()
	ctx, cancel := context.WithTimeout(ctx, timing.CompletionBudget)
	defer cancel()
	ticker := time.NewTicker(timing.PollInterval)
	defer ticker.Stop()
	for {
		latest, err := attestor.LatestHeight(ctx)
		if err == nil && latest >= height {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.Join(fmt.Errorf("attestor latest height %d never reached seed height %d: %w",
				latest, height, ctx.Err()), err)
		case <-ticker.C:
		}
	}
}

// packetCall attests a random seeded packet at its send height.
func packetCall(
	rpc attestorv2.AttestationServiceClient,
	attestor string,
	packets []attestablePacket,
) simpleload.Call {
	return func(ctx context.Context) error {
		packet := packets[rand.IntN(len(packets))]
		res, err := rpc.PacketAttestation(ctx, connect.NewRequest(&attestorv2.PacketAttestationRequest{
			Attestor:       attestor,
			Packets:        [][]byte{packet.encoded},
			Height:         packet.height,
			CommitmentType: attestorv2.CommitmentType_COMMITMENT_TYPE_PACKET,
		}))
		switch {
		case err != nil:
			return err
		case res == nil || res.Msg.Attestation == nil:
			return errors.New("packet attestation: got nil response")
		case res.Msg.Attestation.Height != packet.height:
			return fmt.Errorf("packet attestation: expected height %d, got %d",
				packet.height, res.Msg.Attestation.Height)
		default:
			return nil
		}
	}
}

// stateCall attests a random height in 1..latest.
func stateCall(rpc attestorv2.AttestationServiceClient, attestor string, latest uint64) simpleload.Call {
	return func(ctx context.Context) error {
		height := rand.Uint64N(latest) + 1
		res, err := rpc.StateAttestation(ctx, connect.NewRequest(&attestorv2.StateAttestationRequest{
			Attestor: attestor,
			Height:   height,
		}))
		switch {
		case err != nil:
			return err
		case res == nil || res.Msg.Attestation == nil:
			return errors.New("state attestation: got nil response")
		case res.Msg.Attestation.Height != height:
			return fmt.Errorf("state attestation: expected height %d, got %d", height, res.Msg.Attestation.Height)
		default:
			return nil
		}
	}
}
