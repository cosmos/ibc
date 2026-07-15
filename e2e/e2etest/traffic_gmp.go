package e2etest

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/link/cmd/relayercmd"

	bindings "github.com/cosmos/ibc/link/testappbindings"
)

const (
	eventGMPSent     = "GMPSent"
	eventGMPReceived = "GMPReceived"
)

var (
	gmpABI     = mustABI(bindings.MockGMPMetaData)
	counterABI = mustABI(bindings.CounterMetaData)
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
		return nil, fmt.Errorf("e2etest: pack GMP send: %w", err)
	}
	txHash, sequence, err := send(
		ctx,
		g.source.evm,
		g.sender,
		g.sourceApp,
		data,
		gmpSentSequence(g.sourceApp),
	)
	if err != nil {
		return nil, fmt.Errorf("e2etest: send GMP on route %q: %w", g.routeID, err)
	}
	return &GMPCall{
		app: g,
		packet: Packet{
			RouteID:      g.routeID,
			Source:       g.source.chain.ID(),
			SourceTxHash: txHash,
			Sequence:     sequence,
			appType:      relayercmd.AppTypeGMP,
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
			"e2etest: GMP packet %s target: got %s, want %s",
			c.packet.reference(),
			received.Target.Hex(),
			c.app.counter.Hex(),
		)
	}
	if !received.Success {
		return fmt.Errorf("e2etest: GMP packet %s target call reverted", c.packet.reference())
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
			"e2etest: GMP packet %s target: got %s, want %s",
			c.packet.reference(),
			received.Target.Hex(),
			c.app.counter.Hex(),
		)
	}
	if received.Success {
		return fmt.Errorf(
			"e2etest: GMP packet %s target call succeeded, want rejection",
			c.packet.reference(),
		)
	}
	return c.VerifyTargetUnchanged(ctx)
}

func (g *GMP) count(ctx context.Context) (*big.Int, error) {
	var count *big.Int
	err := g.destination.evm.UseContractCaller(func(caller bind.ContractCaller) error {
		counter, err := bindings.NewCounterCaller(g.counter, caller)
		if err != nil {
			return fmt.Errorf("e2etest: bind Counter %s: %w", g.counter, err)
		}
		count, err = counter.Count(&bind.CallOpts{Context: ctx})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("e2etest: query Counter %s: %w", g.counter, err)
	}
	return count, nil
}

func (c *GMPCall) verifyCount(ctx context.Context, want *big.Int, state string) error {
	got, err := c.app.count(ctx)
	if err != nil {
		return err
	}
	if got.Cmp(want) != 0 {
		return fmt.Errorf(
			"e2etest: GMP packet %s target %s %s count: got %s, want %s",
			c.packet.reference(),
			c.app.counter.Hex(),
			state,
			got,
			want,
		)
	}
	return nil
}

func (c *GMPCall) awaitReceived(ctx context.Context) (*bindings.MockGMPGMPReceived, error) {
	definition := gmpABI.Events[eventGMPReceived]
	parser := mustBinding(bindings.NewMockGMPFilterer(c.app.destApp, nil))
	return awaitPacketEvent(
		ctx,
		eventSource{endpoint: c.app.destination, contract: c.app.destApp},
		definition.ID,
		"GMP",
		func(log types.Log) (*bindings.MockGMPGMPReceived, error) {
			event, err := parser.ParseGMPReceived(log)
			if err != nil {
				return nil, fmt.Errorf("e2etest: decode GMPReceived: %w", err)
			}
			return event, nil
		},
		c.packet,
		func(event *bindings.MockGMPGMPReceived) (string, *big.Int) {
			return event.RouteId, event.Seq
		},
	)
}

func gmpSentSequence(address common.Address) func(*types.Receipt) (uint64, bool, error) {
	parser := mustBinding(bindings.NewMockGMPFilterer(address, nil))
	return func(receipt *types.Receipt) (uint64, bool, error) {
		definition := gmpABI.Events[eventGMPSent]
		for _, log := range receipt.Logs {
			if log.Address != address || len(log.Topics) == 0 || log.Topics[0] != definition.ID {
				continue
			}
			event, err := parser.ParseGMPSent(*log)
			if err != nil {
				return 0, false, fmt.Errorf("e2etest: decode GMPSent: %w", err)
			}
			return event.Seq.Uint64(), true, nil
		}
		return 0, false, nil
	}
}
