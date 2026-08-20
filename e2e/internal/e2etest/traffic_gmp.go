// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics27gmp"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/gen/go/solidity-abi/counter"
)

var gmpTransactor = mustBinding(ics27gmp.NewContractTransactor(common.Address{}, nil))

type GMPRequest struct {
	// Payload defaults to Counter.increment(). Call takes its own copy.
	Payload []byte
	// Receiver defaults to the bound Counter's address.
	Receiver string
	// Salt defaults to empty, matching sendCall's default account identifier.
	Salt []byte
	// Timeout is the destination-relative packet lifetime. Non-positive selects
	// a far-future-but-valid default; positive values are rounded up to whole seconds.
	Timeout time.Duration
}

// GMP drives ICS27 GMP and its default Counter and TestERC20 targets on a
// single directed route.
type GMP struct {
	routeID        RouteID
	source         endpoint
	destination    endpoint
	sender         evm.Account
	sourceGMP      common.Address
	sourceRouter   common.Address
	counter        common.Address
	sourceClientID string
	destClientID   string
	destGMP        common.Address
	destToken      common.Address
	defaultCall    []byte
}

// Token returns the destination-chain TestERC20 bound to this GMP.
func (g *GMP) Token() common.Address { return g.destToken }

// GMPCall is the result of a GMP Call: the packet it emitted plus the Counter
// baseline its Verify methods compare against.
type GMPCall struct {
	sendResult
	app    *GMP
	before *big.Int
}

func (g *GMP) Call(ctx context.Context, request GMPRequest) (*GMPCall, error) {
	payload := request.Payload
	if len(payload) == 0 {
		payload = g.defaultCall
	}
	payload = append([]byte(nil), payload...)
	receiver := request.Receiver
	if receiver == "" {
		receiver = g.counter.Hex()
	}
	before, err := g.count(ctx)
	if err != nil {
		return nil, err
	}
	timeoutTimestamp, err := destinationTimeout(ctx, g.destination, request.Timeout)
	if err != nil {
		return nil, err
	}
	msg := ics27gmp.IICS27GMPMsgsSendCallMsg{
		SourceClient:     g.sourceClientID,
		Receiver:         receiver,
		Salt:             request.Salt,
		Payload:          payload,
		TimeoutTimestamp: timeoutTimestamp,
		Memo:             "",
	}
	data, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return gmpTransactor.SendCall(opts, msg)
	})
	if err != nil {
		return nil, fmt.Errorf("e2etest: pack GMP sendCall: %w", err)
	}
	receipt, sequence, err := send(
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
		sendResult: newSendResult(g.routeID, g.source, g.sourceClientID, receipt, sequence),
		app:        g,
		before:     before,
	}, nil
}

// VerifyCounterExecuted waits for the destination Counter to change exactly
// once. Only meaningful when the call's receiver was the bound Counter.
func (c *GMPCall) VerifyCounterExecuted(ctx context.Context) error {
	want := new(big.Int).Add(c.before, big.NewInt(1))
	return awaitBalance(
		ctx,
		c.app.destination.chain,
		fmt.Sprintf("GMP packet %s Counter execution", c.packetTx.reference()),
		c.app.count,
		want,
	)
}

// VerifyCounterUnchanged checks that the destination Counter did not change.
// Only meaningful when the call's receiver was the bound Counter.
func (c *GMPCall) VerifyCounterUnchanged(ctx context.Context) error {
	return c.verifyCount(ctx, c.before, "unchanged")
}

// VerifyTimeoutExecuted checks that txHash succeeded with a TimeoutPacket for this packet.
func (c *GMPCall) VerifyTimeoutExecuted(ctx context.Context, txHash string) error {
	return verifyPacketTimeout(
		ctx, c.app.source, c.app.sourceRouter, c.app.sourceClientID, c.packetTx, txHash,
	)
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

// AccountIdentifier builds the ICS27 account identifier that onRecvPacket
// constructs on the destination chain for a call sent by sender with salt:
func (g *GMP) AccountIdentifier(sender common.Address, salt []byte) ics27gmp.IICS27GMPMsgsAccountIdentifier {
	return ics27gmp.IICS27GMPMsgsAccountIdentifier{
		ClientId: g.destClientID,
		Sender:   sender.Hex(),
		Salt:     salt,
	}
}

// AccountAddress derives the ICS27 account address for id
func (g *GMP) AccountAddress(
	ctx context.Context,
	id ics27gmp.IICS27GMPMsgsAccountIdentifier,
) (common.Address, error) {
	var address common.Address
	err := g.destination.evm.UseContractCaller(func(caller bind.ContractCaller) error {
		bound, err := ics27gmp.NewContractCaller(g.destGMP, caller)
		if err != nil {
			return fmt.Errorf("e2etest: bind ICS27GMP %s: %w", g.destGMP, err)
		}
		address, err = bound.GetOrComputeAccountAddress(&bind.CallOpts{Context: ctx}, id)
		return err
	})
	if err != nil {
		return common.Address{}, fmt.Errorf("e2etest: compute ICS27 account address: %w", err)
	}
	return address, nil
}

// StoredAccountIdentifier reads back the account identifier the destination
// ICS27GMP contract recorded for account.
func (g *GMP) StoredAccountIdentifier(
	ctx context.Context,
	account common.Address,
) (ics27gmp.IICS27GMPMsgsAccountIdentifier, error) {
	var id ics27gmp.IICS27GMPMsgsAccountIdentifier
	err := g.destination.evm.UseContractCaller(func(caller bind.ContractCaller) error {
		bound, err := ics27gmp.NewContractCaller(g.destGMP, caller)
		if err != nil {
			return fmt.Errorf("e2etest: bind ICS27GMP %s: %w", g.destGMP, err)
		}
		id, err = bound.GetAccountIdentifier(&bind.CallOpts{Context: ctx}, account)
		return err
	})
	if err != nil {
		return ics27gmp.IICS27GMPMsgsAccountIdentifier{}, fmt.Errorf(
			"e2etest: query ICS27 account identifier for %s: %w", account, err,
		)
	}
	return id, nil
}

// ERC20BalanceOf queries the bound TestERC20's balance on the destination chain.
func (g *GMP) ERC20BalanceOf(ctx context.Context, holder common.Address) (*big.Int, error) {
	return erc20BalanceOf(ctx, g.destination.evm, g.destToken, holder)
}

// FundERC20 mints amount of the bound TestERC20 to holder on the destination
// chain. holder need not have any code deployed yet: minting only writes a
// balance entry.
func (g *GMP) FundERC20(ctx context.Context, holder common.Address, amount *big.Int) error {
	data, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return testERC20Transactor.Mint(opts, holder, amount)
	})
	if err != nil {
		return fmt.Errorf("e2etest: pack TestERC20.mint: %w", err)
	}
	if _, err := g.destination.evm.BroadcastTx(ctx, g.sender, &g.destToken, data, nil); err != nil {
		return fmt.Errorf("e2etest: fund %s with TestERC20 %s: %w", holder, g.destToken, err)
	}
	return nil
}

// AwaitERC20Balance waits until the bound TestERC20's holder balance equals want.
func (g *GMP) AwaitERC20Balance(
	ctx context.Context,
	holder common.Address,
	want *big.Int,
	description string,
) error {
	return awaitBalance(
		ctx,
		g.destination.chain,
		description,
		func(ctx context.Context) (*big.Int, error) { return g.ERC20BalanceOf(ctx, holder) },
		want,
	)
}

// PackERC20Transfer ABI-encodes an erc20.transfer(to, amount) call, for use as
// a GMPRequest.Payload delivered to an ICS27 account.
func PackERC20Transfer(to common.Address, amount *big.Int) ([]byte, error) {
	data, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return testERC20Transactor.Transfer(opts, to, amount)
	})
	if err != nil {
		return nil, fmt.Errorf("e2etest: pack TestERC20.transfer: %w", err)
	}
	return data, nil
}

func (c *GMPCall) verifyCount(ctx context.Context, want *big.Int, state string) error {
	got, err := c.app.count(ctx)
	if err != nil {
		return err
	}
	if got.Cmp(want) != 0 {
		return fmt.Errorf(
			"e2etest: GMP packet %s target %s %s count: got %s, want %s",
			c.packetTx.reference(),
			c.app.counter.Hex(),
			state,
			got,
			want,
		)
	}
	return nil
}
