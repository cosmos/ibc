package e2etest

import (
	"context"
	"fmt"
	"math/big"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics27gmp"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/counter"
)

var gmpABI = mustABI(ics27gmp.ContractMetaData)

type GMPRequest struct {
	// Payload defaults to Counter.increment(). Call takes its own copy.
	Payload []byte
}

// GMP binds ICS27 GMP and its default Counter target on a single directed route.
type GMP struct {
	routeID      RouteID
	source       endpoint
	destination  endpoint
	sender       evm.Account
	sourceGMP    common.Address
	sourceRouter common.Address
	counter      common.Address
	sourceClient string
	defaultCall  []byte
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
	timeoutTimestamp, err := destinationTimeout(ctx, g.destination, 0)
	if err != nil {
		return nil, err
	}
	msg := ics27gmp.IICS27GMPMsgsSendCallMsg{
		SourceClient:     g.sourceClient,
		Receiver:         g.counter.Hex(),
		Salt:             nil,
		Payload:          payload,
		TimeoutTimestamp: timeoutTimestamp,
		Memo:             "",
	}
	data, err := gmpABI.Pack("sendCall", msg)
	if err != nil {
		return nil, fmt.Errorf("e2etest: pack GMP sendCall: %w", err)
	}
	txHash, sequence, err := send(
		ctx,
		g.source.evm,
		g.sender,
		g.sourceGMP,
		data,
		sendPacketSequence(g.sourceRouter),
	)
	if err != nil {
		return nil, fmt.Errorf("e2etest: send GMP on route %q: %w", g.routeID, err)
	}
	return &GMPCall{
		app: g,
		packet: Packet{
			RouteID:      g.routeID,
			Source:       g.source.chain.ID(),
			SourceClient: g.sourceClient,
			SourceTxHash: txHash,
			Sequence:     sequence,
		},
		before: before,
	}, nil
}

func (c *GMPCall) Packet() Packet { return c.packet }

// VerifyExecuted waits for the destination Counter to change exactly once.
func (c *GMPCall) VerifyExecuted(ctx context.Context) error {
	want := new(big.Int).Add(c.before, big.NewInt(1))
	return awaitBalance(
		ctx,
		c.app.destination.chain,
		fmt.Sprintf("GMP packet %s Counter execution", c.packet.reference()),
		c.app.count,
		want,
	)
}

func (c *GMPCall) VerifyTargetUnchanged(ctx context.Context) error {
	return c.verifyCount(ctx, c.before, "unchanged")
}

// VerifyRejected checks that the target state did not change after an error acknowledgement.
func (c *GMPCall) VerifyRejected(ctx context.Context) error {
	return c.VerifyTargetUnchanged(ctx)
}

func (g *GMP) count(ctx context.Context) (*big.Int, error) {
	var count *big.Int
	err := g.destination.evm.UseContractCaller(func(caller bind.ContractCaller) error {
		bound, err := counter.NewCounterCaller(g.counter, caller)
		if err != nil {
			return fmt.Errorf("e2etest: bind Counter %s: %w", g.counter, err)
		}
		count, err = bound.Count(&bind.CallOpts{Context: ctx})
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
