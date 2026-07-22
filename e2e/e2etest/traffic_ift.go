package e2etest

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ift"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
)

var iftABI = mustABI(ift.ContractMetaData)

type IFTRequest struct {
	// Amount must be positive and fit in a uint256. Send takes its own copy.
	Amount *big.Int
	// Receiver defaults to a fresh address on the destination Chain.
	Receiver string
	// Timeout is the destination-relative packet lifetime. Non-positive selects
	// a far-future-but-valid default; positive values are rounded up to whole seconds.
	Timeout time.Duration
}

// IFT binds the Interchain Fungible Token pair on a single directed route.
type IFT struct {
	routeID      RouteID
	source       endpoint
	destination  endpoint
	sender       evm.Account
	sourceIFT    common.Address
	destIFT      common.Address
	sourceRouter common.Address
	sourceClient string
}

type IFTPacket struct {
	app               *IFT
	packet            Packet
	receiver          common.Address
	amount            *big.Int
	sourceBefore      *big.Int
	destinationBefore *big.Int
}

func (i *IFT) Send(ctx context.Context, request IFTRequest) (*IFTPacket, error) {
	amount, err := validAmount(request.Amount)
	if err != nil {
		return nil, err
	}
	receiver, err := i.receiver(request.Receiver)
	if err != nil {
		return nil, err
	}
	sourceBefore, err := i.balance(ctx, i.source.evm, i.sourceIFT, i.sender.Address())
	if err != nil {
		return nil, err
	}
	destinationBefore, err := i.balance(ctx, i.destination.evm, i.destIFT, receiver)
	if err != nil {
		return nil, err
	}
	timeoutTimestamp, err := destinationTimeout(ctx, i.destination, request.Timeout)
	if err != nil {
		return nil, err
	}
	data, err := iftABI.Pack("iftTransfer", i.sourceClient, receiver.Hex(), amount, timeoutTimestamp)
	if err != nil {
		return nil, fmt.Errorf("e2etest: pack IFT iftTransfer: %w", err)
	}
	txHash, sequence, err := send(
		ctx,
		i.source.evm,
		i.sender,
		i.sourceIFT,
		data,
		sendPacketSequence(i.sourceRouter),
	)
	if err != nil {
		return nil, fmt.Errorf("e2etest: send IFT on route %q: %w", i.routeID, err)
	}
	return &IFTPacket{
		app: i,
		packet: Packet{
			RouteID:      i.routeID,
			Source:       i.source.chain.ID(),
			SourceTxHash: txHash,
			Sequence:     sequence,
		},
		receiver:          receiver,
		amount:            amount,
		sourceBefore:      sourceBefore,
		destinationBefore: destinationBefore,
	}, nil
}

func (p *IFTPacket) Packet() Packet { return p.packet }

// VerifyBurned checks that the submitted amount left the source sender balance.
func (p *IFTPacket) VerifyBurned(ctx context.Context) error {
	want := new(big.Int).Sub(p.sourceBefore, p.amount)
	got, err := p.app.balance(ctx, p.app.source.evm, p.app.sourceIFT, p.app.sender.Address())
	if err != nil {
		return err
	}
	if got.Cmp(want) != 0 {
		return fmt.Errorf(
			"e2etest: IFT packet %s burned balance of %s: got %s, want %s",
			p.packet.reference(),
			p.app.sender.Address().Hex(),
			got,
			want,
		)
	}
	return nil
}

// VerifyDelivered waits for the destination IFT to mint the submitted amount
// at the submitted receiver.
func (p *IFTPacket) VerifyDelivered(ctx context.Context) error {
	want := new(big.Int).Add(p.destinationBefore, p.amount)
	return awaitBalance(
		ctx,
		p.app.destination.chain,
		fmt.Sprintf("IFT packet %s mint delivery", p.packet.reference()),
		func(ctx context.Context) (*big.Int, error) {
			return p.app.balance(ctx, p.app.destination.evm, p.app.destIFT, p.receiver)
		},
		want,
	)
}

func (p *IFTPacket) VerifyNotMinted(ctx context.Context) error {
	got, err := p.app.balance(ctx, p.app.destination.evm, p.app.destIFT, p.receiver)
	if err != nil {
		return err
	}
	if got.Cmp(p.destinationBefore) != 0 {
		return fmt.Errorf(
			"e2etest: IFT packet %s not minted balance of %s: got %s, want %s",
			p.packet.reference(),
			p.receiver.Hex(),
			got,
			p.destinationBefore,
		)
	}
	return nil
}

// VerifyRefunded waits for the source sender balance to be restored after timeout.
func (p *IFTPacket) VerifyRefunded(ctx context.Context) error {
	if err := awaitPacketTimeout(ctx, p.app.source, p.app.sourceRouter, p.app.sourceClient, p.packet); err != nil {
		return err
	}
	return awaitBalance(
		ctx,
		p.app.source.chain,
		fmt.Sprintf("IFT packet %s refund", p.packet.reference()),
		func(ctx context.Context) (*big.Int, error) {
			return p.app.balance(ctx, p.app.source.evm, p.app.sourceIFT, p.app.sender.Address())
		},
		p.sourceBefore,
	)
}

func (i *IFT) receiver(value string) (common.Address, error) {
	if value != "" {
		if !common.IsHexAddress(value) {
			return common.Address{}, fmt.Errorf("e2etest: IFT receiver %q is not an EVM address", value)
		}
		return common.HexToAddress(value), nil
	}
	account, err := evm.NewAccount()
	if err != nil {
		return common.Address{}, fmt.Errorf("e2etest: generate IFT receiver: %w", err)
	}
	return account.Address(), nil
}

func (i *IFT) balance(
	ctx context.Context,
	client *environment.EVM,
	contract common.Address,
	holder common.Address,
) (*big.Int, error) {
	var balance *big.Int
	err := client.UseContractCaller(func(caller bind.ContractCaller) error {
		bound, err := ift.NewContractCaller(contract, caller)
		if err != nil {
			return fmt.Errorf("e2etest: bind IFT %s: %w", contract, err)
		}
		balance, err = bound.BalanceOf(&bind.CallOpts{Context: ctx}, holder)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("e2etest: query IFT balance of %s: %w", holder.Hex(), err)
	}
	return balance, nil
}
