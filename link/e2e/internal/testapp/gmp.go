package testapp

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/link/e2e/internal/testapp/contracts"
	"github.com/cosmos/ibc/link/harness/chain/evm"
	"github.com/cosmos/ibc/link/harness/environment"
)

const (
	eventGMPSent     = "GMPSent"
	eventGMPReceived = "GMPReceived"
)

var (
	gmpABI     = contracts.MockGMP.MustABI()
	counterABI = contracts.Counter.MustABI()
)

type GMPRequest struct {
	// Payload defaults to Counter.increment(). Call takes its own copy.
	Payload []byte
}

// GMP binds MockGMP and its default Counter target on a single directed route.
type GMP struct {
	routeID     RouteID
	source      endpoint
	destination endpoint
	sender      evm.Account
	sourceApp   common.Address
	destApp     common.Address
	counter     common.Address
	defaultCall []byte
}

func NewGMP(
	routeID RouteID,
	source, destination *environment.Chain,
	sender evm.Account,
	contracts GMPContracts,
) (*GMP, error) {
	sourceEndpoint, destinationEndpoint, err := bindRoute(routeID, source, destination)
	if err != nil {
		return nil, err
	}
	sourceApp, err := address("source MockGMP", contracts.Source)
	if err != nil {
		return nil, err
	}
	destinationApp, err := address("destination MockGMP", contracts.Destination)
	if err != nil {
		return nil, err
	}
	counter, err := address("destination Counter", contracts.Counter)
	if err != nil {
		return nil, err
	}
	defaultCall, err := counterABI.Pack("increment")
	if err != nil {
		return nil, fmt.Errorf("testapp: pack Counter.increment(): %w", err)
	}
	return &GMP{
		routeID:     routeID,
		source:      sourceEndpoint,
		destination: destinationEndpoint,
		sender:      sender,
		sourceApp:   sourceApp,
		destApp:     destinationApp,
		counter:     counter,
		defaultCall: defaultCall,
	}, nil
}

type GMPCall struct {
	app    *GMP
	packet Packet
	before *big.Int
}

func (g *GMP) Call(ctx context.Context, request GMPRequest) (*GMPCall, error) {
	payload := request.Payload
	if len(payload) == 0 {
		payload = g.defaultCall
	}
	payload = append([]byte(nil), payload...)
	before, err := g.count(ctx)
	if err != nil {
		return nil, err
	}
	data, err := gmpABI.Pack("send", string(g.routeID), g.counter.Hex(), payload)
	if err != nil {
		return nil, fmt.Errorf("testapp: pack GMP send: %w", err)
	}
	txHash, sequence, err := send(
		ctx,
		g.source.evm,
		g.sender,
		g.sourceApp,
		data,
		gmpABI,
		eventGMPSent,
	)
	if err != nil {
		return nil, fmt.Errorf("testapp: send GMP on route %q: %w", g.routeID, err)
	}
	return &GMPCall{
		app: g,
		packet: Packet{
			RouteID:      g.routeID,
			Source:       g.source.chain.ID(),
			SourceTxHash: txHash,
			Sequence:     sequence,
			application:  ApplicationGMP,
		},
		before: before,
	}, nil
}

func (c *GMPCall) Packet() Packet { return c.packet }

// VerifyExecuted waits for the destination effect and checks that the Counter
// changed exactly once.
func (c *GMPCall) VerifyExecuted(ctx context.Context) error {
	received, err := c.awaitReceived(ctx)
	if err != nil {
		return err
	}
	if received.Target != c.app.counter {
		return fmt.Errorf(
			"testapp: GMP packet %s target: got %s, want %s",
			c.packet.reference(),
			received.Target.Hex(),
			c.app.counter.Hex(),
		)
	}
	if !received.Success {
		return fmt.Errorf("testapp: GMP packet %s target call reverted", c.packet.reference())
	}
	want := new(big.Int).Add(c.before, big.NewInt(1))
	return c.verifyCount(ctx, want, "executed")
}

func (c *GMPCall) VerifyTargetUnchanged(ctx context.Context) error {
	return c.verifyCount(ctx, c.before, "unchanged")
}

// VerifyRejected waits for the destination application to reject the payload
// and checks that the target state did not change.
func (c *GMPCall) VerifyRejected(ctx context.Context) error {
	received, err := c.awaitReceived(ctx)
	if err != nil {
		return err
	}
	if received.Target != c.app.counter {
		return fmt.Errorf(
			"testapp: GMP packet %s target: got %s, want %s",
			c.packet.reference(),
			received.Target.Hex(),
			c.app.counter.Hex(),
		)
	}
	if received.Success {
		return fmt.Errorf(
			"testapp: GMP packet %s target call succeeded, want rejection",
			c.packet.reference(),
		)
	}
	return c.VerifyTargetUnchanged(ctx)
}

func (g *GMP) count(ctx context.Context) (*big.Int, error) {
	return callUint(ctx, g.destination.evm, counterABI, g.counter, "count")
}

func (c *GMPCall) verifyCount(ctx context.Context, want *big.Int, state string) error {
	got, err := c.app.count(ctx)
	if err != nil {
		return err
	}
	if got.Cmp(want) != 0 {
		return fmt.Errorf(
			"testapp: GMP packet %s target %s %s count: got %s, want %s",
			c.packet.reference(),
			c.app.counter.Hex(),
			state,
			got,
			want,
		)
	}
	return nil
}

type gmpReceivedLog struct {
	RouteID string `abi:"routeId"`
	Seq     *big.Int
	Target  common.Address
	Success bool
}

func (c *GMPCall) awaitReceived(ctx context.Context) (gmpReceivedLog, error) {
	definition := gmpABI.Events[eventGMPReceived]
	description := fmt.Sprintf("GMP delivery for packet %s", c.packet.reference())
	return awaitEvent(
		ctx,
		c.app.destination.chain,
		c.app.destination.evm,
		c.app.destApp,
		definition.ID,
		description,
		func(data []byte) (gmpReceivedLog, error) {
			var event gmpReceivedLog
			if err := gmpABI.UnpackIntoInterface(&event, eventGMPReceived, data); err != nil {
				return gmpReceivedLog{}, fmt.Errorf("testapp: decode GMPReceived: %w", err)
			}
			return event, nil
		},
		func(event gmpReceivedLog) bool {
			return event.RouteID == string(c.packet.RouteID) && event.Seq.Uint64() == c.packet.Sequence
		},
	)
}
