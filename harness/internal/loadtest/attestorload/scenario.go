// SPDX-License-Identifier: Apache-2.0

// Package attestorload is the attestor load scenario: fixed-rate packet and
// state attestation requests against the gRPC API of the Attestor that
// observes Chain A, over packets the relayer already delivered.
package attestorload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/common"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	attestorv2 "github.com/cosmos/ibc/cli/api/v2/attestor"
	relayerv2 "github.com/cosmos/ibc/cli/api/v2/relayer"
	attestoribc "github.com/cosmos/ibc/cli/attestor/evm/ibc"
	"github.com/cosmos/ibc/harness/environment"
	"github.com/cosmos/ibc/harness/internal/loadtest/session"
	"github.com/cosmos/ibc/harness/internal/loadtest/simpleload"
	"github.com/cosmos/ibc/harness/internal/loadtest/textout"
	"github.com/cosmos/ibc/harness/traffic"
)

const (
	// seedPackets is the number of delivered packets the packet phase
	// attests at random.
	seedPackets  = 10
	seedAmount   = 100_000
	PhasePackets = "packets"
	PhaseState   = "state"
)

// Config shapes one attestor scenario run.
type Config struct {
	Packets simpleload.Spec
	State   simpleload.Spec
	// Phases selects which of PhasePackets and PhaseState run, in order.
	Phases  []string
	Session session.Options
}

// PhaseReport is the outcome of one phase.
type PhaseReport struct {
	Phase  string             `json:"phase"`
	Result simpleload.Summary `json:"result"`
	Passed bool               `json:"passed"`
}

// Report is the outcome of the attestor scenario.
type Report struct {
	Scenario string        `json:"scenario"`
	Provider string        `json:"provider"`
	Attestor string        `json:"attestor"`
	Phases   []PhaseReport `json:"phases"`
	Passed   bool          `json:"passed"`
}

// WriteText renders the report for humans.
func (r *Report) WriteText(w io.Writer) error {
	out := textout.New(w)
	verdict := "PASS"
	if !r.Passed {
		verdict = "FAIL"
	}
	out.Printf("\nAttestor load: %s (%s, attestor %s)\n", verdict, r.Provider, r.Attestor)
	for _, phase := range r.Phases {
		res := phase.Result
		out.Printf("\n[%s] %s at %d rps, concurrency %d\n", phase.Phase,
			time.Duration(res.DurationMS)*time.Millisecond, res.RatePerSecond, res.Concurrency)
		out.Printf("  requests: succeeded=%d failed=%d timed_out=%d dropped=%d\n",
			res.Succeeded, res.Failed, res.TimedOut, res.Dropped)
		l := res.Latency
		out.Printf("  latency:  n=%d mean=%.1fms p50=%.1fms p90=%.1fms p95=%.1fms p99=%.1fms max=%.1fms\n",
			l.Count, l.MeanMS, l.P50MS, l.P90MS, l.P95MS, l.P99MS, l.MaxMS)
	}
	return out.Err()
}

type attestablePacket struct {
	height  uint64
	encoded []byte
}

// Run executes the scenario.
func Run(ctx context.Context, cfg Config) (report *Report, err error) {
	opts := cfg.Session
	route := traffic.AtoB(traffic.ChainA, traffic.ChainB)
	opts.Routes = []traffic.Route{route}
	s, err := session.Start(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer func() {
		var failure error
		if err != nil || report == nil || !report.Passed {
			failure = errors.New("scenario failed")
		}
		err = errors.Join(err, s.Close(ctx, failure))
	}()

	attestor, err := observingAttestor(s.Env, traffic.ChainA)
	if err != nil {
		return nil, err
	}
	packets, err := seed(ctx, s, route)
	if err != nil {
		return nil, err
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

	report = &Report{
		Scenario: "attestor",
		Provider: string(opts.Provider),
		Attestor: name,
		Passed:   true,
	}
	for _, phase := range cfg.Phases {
		var spec simpleload.Spec
		var call simpleload.Call
		switch phase {
		case PhasePackets:
			spec, call = cfg.Packets, packetCall(rpc, name, packets)
		case PhaseState:
			latest, heightErr := attestor.LatestHeight(ctx)
			if heightErr != nil {
				return report, fmt.Errorf("attestor latest height: %w", heightErr)
			}
			if latest == 0 {
				return report, errors.New("attestor reports no attestable height")
			}
			spec, call = cfg.State, stateCall(rpc, name, latest)
		default:
			return report, fmt.Errorf("unknown phase %q", phase)
		}
		slog.InfoContext(ctx, "attestor load phase", "phase", phase, "duration", spec.Duration,
			"rps", spec.RatePerSecond, "concurrency", spec.Concurrency)
		result := simpleload.SimpleLoad(ctx, spec, call)
		if err := ctx.Err(); err != nil {
			return report, err
		}
		phaseReport := PhaseReport{Phase: phase, Result: result.Summary(), Passed: result.Succeeded > 0}
		report.Phases = append(report.Phases, phaseReport)
		report.Passed = report.Passed && phaseReport.Passed
	}
	return report, nil
}

func observingAttestor(env *environment.Environment, chain environment.ChainID) (*environment.Attestor, error) {
	for _, id := range env.Attestors() {
		candidate, err := env.Attestor(id)
		if err != nil {
			return nil, err
		}
		if candidate.ObservedIBCInstance().Chain().ID() == chain {
			return candidate, nil
		}
	}
	return nil, fmt.Errorf("no attestor observes %s", chain)
}

// seed sends transfers on route and waits until the relayer delivered them,
// so their commitments are attestable at their send heights.
func seed(ctx context.Context, s *session.Session, route traffic.Route) ([]attestablePacket, error) {
	app, err := traffic.NewTransfer(s.Env, s.Deployment, s.Deployer, route)
	if err != nil {
		return nil, err
	}
	sends := make([]*traffic.TransferSend, 0, seedPackets)
	for range seedPackets {
		send, sendErr := app.Send(ctx, traffic.TransferRequest{Amount: big.NewInt(seedAmount)})
		if sendErr != nil {
			return nil, fmt.Errorf("seed transfer: %w", sendErr)
		}
		sends = append(sends, send)
	}
	errs := make([]error, len(sends))
	var wg sync.WaitGroup
	for i, send := range sends {
		wg.Go(func() {
			_, errs[i] = traffic.AwaitState(
				ctx, s.Relayer, send.PacketTx(), relayerv2.PacketState_PACKET_STATE_SUCCEEDED,
			)
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("seed delivery: %w", err)
	}
	slog.InfoContext(ctx, "seeded delivered packets", "count", len(sends))

	source, ok := s.Deployment.Chain(route.Source)
	if !ok {
		return nil, fmt.Errorf("no deployment on Chain %q", route.Source)
	}
	packets := make([]attestablePacket, 0, len(sends))
	for _, send := range sends {
		packet, err := newAttestablePacket(send, source.ICS26Router)
		if err != nil {
			return nil, err
		}
		packets = append(packets, packet)
	}
	return packets, nil
}

func newAttestablePacket(send *traffic.TransferSend, router common.Address) (attestablePacket, error) {
	tx := send.PacketTx()
	sent, err := traffic.SentPackets(send.Receipt(), router)
	if err != nil {
		return attestablePacket{}, err
	}
	for _, packet := range sent {
		if packet.SourceClient != tx.SourceClientID || packet.Sequence != tx.Sequence {
			continue
		}
		encoded, err := attestoribc.EncodePacket(routerPacket(packet))
		if err != nil {
			return attestablePacket{}, fmt.Errorf("encode packet %s: %w", tx, err)
		}
		return attestablePacket{height: tx.SourceBlockNumber, encoded: encoded}, nil
	}
	return attestablePacket{}, fmt.Errorf("no matching SendPacket event for packet %s", tx)
}

func routerPacket(p ics26router.IICS26RouterMsgsPacket) channeltypesv2.Packet {
	payloads := make([]channeltypesv2.Payload, len(p.Payloads))
	for i, item := range p.Payloads {
		payloads[i] = channeltypesv2.Payload{
			SourcePort:      item.SourcePort,
			DestinationPort: item.DestPort,
			Version:         item.Version,
			Encoding:        item.Encoding,
			Value:           item.Value,
		}
	}
	return channeltypesv2.Packet{
		Sequence:          p.Sequence,
		SourceClient:      p.SourceClient,
		DestinationClient: p.DestClient,
		TimeoutTimestamp:  p.TimeoutTimestamp,
		Payloads:          payloads,
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
			return fmt.Errorf(
				"packet attestation: expected height %d, got %d",
				packet.height,
				res.Msg.Attestation.Height,
			)
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
