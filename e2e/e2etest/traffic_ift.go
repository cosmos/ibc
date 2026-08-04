package e2etest

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ift"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/iftbatchtransfershim"
)

var (
	iftABI          = mustABI(ift.ContractMetaData)
	iftBatchShimABI = mustABI(iftbatchtransfershim.IFTBatchTransferShimMetaData)
)

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
	batcher      common.Address
}

type IFTPacket struct {
	app                     *IFT
	packet                  Packet
	receiver                common.Address
	amount                  *big.Int
	sourceBefore            *big.Int
	destinationBefore       *big.Int
	destinationSupplyBefore *big.Int
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
	destinationSupplyBefore, err := i.totalSupply(ctx, i.destination.evm, i.destIFT)
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
			SourceClient: i.sourceClient,
			SourceTxHash: txHash,
			Sequence:     sequence,
		},
		receiver:                receiver,
		amount:                  amount,
		sourceBefore:            sourceBefore,
		destinationBefore:       destinationBefore,
		destinationSupplyBefore: destinationSupplyBefore,
	}, nil
}

// SendBatch emits multiple IFT transfers from a single source transaction
func (i *IFT) SendBatch(ctx context.Context, requests []IFTRequest) (*IFTBatch, error) {
	batcherBefore, balanceErr := i.balance(ctx, i.source.evm, i.sourceIFT, i.batcher)
	if balanceErr != nil {
		return nil, balanceErr
	}

	transfers := make([]iftbatchtransfershim.IFTBatchTransferShimTransfer, len(requests))
	destinationsBefore := make([]*big.Int, len(requests))
	seenReceivers := make(map[common.Address]struct{}, len(requests))
	total := big.NewInt(0)
	for k, request := range requests {
		amount, err := validAmount(request.Amount)
		if err != nil {
			return nil, err
		}
		receiver, err := i.receiver(request.Receiver)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seenReceivers[receiver]; duplicate {
			return nil, fmt.Errorf(
				"e2etest: IFT batch on route %q has duplicate receiver %s: "+
					"per-packet VerifyDelivered assumes each receiver appears once in the batch",
				i.routeID, receiver.Hex(),
			)
		}
		seenReceivers[receiver] = struct{}{}
		destinationBefore, err := i.balance(ctx, i.destination.evm, i.destIFT, receiver)
		if err != nil {
			return nil, err
		}
		timeoutTimestamp, err := destinationTimeout(ctx, i.destination, request.Timeout)
		if err != nil {
			return nil, err
		}
		transfers[k] = iftbatchtransfershim.IFTBatchTransferShimTransfer{
			Receiver:         receiver.Hex(),
			Amount:           amount,
			TimeoutTimestamp: timeoutTimestamp,
		}
		destinationsBefore[k] = destinationBefore
		total.Add(total, amount)
	}

	data, err := iftBatchShimABI.Pack("batchIftTransfer", i.sourceIFT, i.sourceClient, transfers)
	if err != nil {
		return nil, fmt.Errorf("e2etest: pack IFT batchIftTransfer: %w", err)
	}
	receipt, err := i.source.evm.BroadcastTx(ctx, i.sender, &i.batcher, data, nil)
	if err != nil {
		return nil, fmt.Errorf("e2etest: send IFT batch on route %q: %w", i.routeID, err)
	}
	sequences, err := sendPacketSequences(i.sourceRouter, receipt)
	if err != nil {
		return nil, err
	}
	if len(sequences) != len(requests) {
		return nil, fmt.Errorf(
			"e2etest: IFT batch on route %q emitted %d packets, want %d",
			i.routeID,
			len(sequences),
			len(requests),
		)
	}

	packets := make([]*IFTPacket, len(requests))
	for k, sequence := range sequences {
		packets[k] = &IFTPacket{
			app: i,
			packet: Packet{
				RouteID:      i.routeID,
				Source:       i.source.chain.ID(),
				SourceClient: i.sourceClient,
				SourceTxHash: receipt.TxHash.Hex(),
				Sequence:     sequence,
			},
			receiver:          common.HexToAddress(transfers[k].Receiver),
			amount:            transfers[k].Amount,
			destinationBefore: destinationsBefore[k],
		}
	}
	return &IFTBatch{app: i, packets: packets, batcherBefore: batcherBefore, total: total}, nil
}

// IFTBatch is the result of a SendBatch call: several IFT packets emitted
// from a single source transaction.
type IFTBatch struct {
	app           *IFT
	packets       []*IFTPacket
	batcherBefore *big.Int
	total         *big.Int
}

func (b *IFTBatch) Packets() []Packet {
	packets := make([]Packet, len(b.packets))
	for k, packet := range b.packets {
		packets[k] = packet.Packet()
	}
	return packets
}

func (b *IFTBatch) VerifyDelivered(ctx context.Context) error {
	for _, packet := range b.packets {
		if err := packet.VerifyDelivered(ctx); err != nil {
			return err
		}
	}
	return nil
}

// VerifyBurned checks that the batcher's balance decreased by the sum of the
// batch's amounts.
func (b *IFTBatch) VerifyBurned(ctx context.Context) error {
	want := new(big.Int).Sub(b.batcherBefore, b.total)
	got, err := b.app.balance(ctx, b.app.source.evm, b.app.sourceIFT, b.app.batcher)
	if err != nil {
		return err
	}
	if got.Cmp(want) != 0 {
		return fmt.Errorf(
			"e2etest: IFT batch batcher %s burned balance: got %s, want %s",
			b.app.batcher.Hex(),
			got,
			want,
		)
	}
	return nil
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
	supply, err := p.app.totalSupply(ctx, p.app.destination.evm, p.app.destIFT)
	if err != nil {
		return err
	}
	if supply.Cmp(p.destinationSupplyBefore) != 0 {
		return fmt.Errorf(
			"e2etest: IFT packet %s destination total supply: got %s, want %s",
			p.packet.reference(),
			supply,
			p.destinationSupplyBefore,
		)
	}
	return nil
}

// VerifyRefunded waits for the source sender balance to be restored after the transfer completes.
func (p *IFTPacket) VerifyRefunded(ctx context.Context) error {
	if err := p.awaitPendingCleared(ctx); err != nil {
		return err
	}
	return p.awaitSourceRefund(ctx)
}

func (p *IFTPacket) awaitPendingCleared(ctx context.Context) error {
	timing := p.app.source.chain.Timing()
	_, err := await(
		ctx,
		timing.CompletionBudget,
		timing.PollInterval,
		fmt.Sprintf("IFT packet %s pending transfer cleared", p.packet.reference()),
		func(ctx context.Context) (struct{}, bool, error) {
			_, err := p.app.pendingTransfer(
				ctx,
				p.app.source.evm,
				p.app.sourceIFT,
				p.app.sourceClient,
				p.packet.Sequence,
			)
			switch {
			case err == nil:
				return struct{}{}, false, fmt.Errorf("pending transfer still present")
			case isIFTPendingTransferNotFound(err):
				return struct{}{}, true, nil
			default:
				return struct{}{}, false, err
			}
		},
	)
	return err
}

func (p *IFTPacket) awaitSourceRefund(ctx context.Context) error {
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

// VerifyPending checks that a pending-transfer record exists on the source
// IFT contract for this packet's sequence, with the expected sender and amount.
func (p *IFTPacket) VerifyPending(ctx context.Context) error {
	record, err := p.app.pendingTransfer(ctx, p.app.source.evm, p.app.sourceIFT, p.app.sourceClient, p.packet.Sequence)
	if err != nil {
		return fmt.Errorf("e2etest: IFT packet %s pending transfer: %w", p.packet.reference(), err)
	}
	if record.Sender != p.app.sender.Address() {
		return fmt.Errorf(
			"e2etest: IFT packet %s pending transfer sender: got %s, want %s",
			p.packet.reference(),
			record.Sender.Hex(),
			p.app.sender.Address().Hex(),
		)
	}
	if record.Amount.Cmp(p.amount) != 0 {
		return fmt.Errorf(
			"e2etest: IFT packet %s pending transfer amount: got %s, want %s",
			p.packet.reference(),
			record.Amount,
			p.amount,
		)
	}
	return nil
}

// VerifyPendingCleared checks that no pending-transfer record exists for
// this packet's sequence on the source IFT contract.
func (p *IFTPacket) VerifyPendingCleared(ctx context.Context) error {
	_, err := p.app.pendingTransfer(ctx, p.app.source.evm, p.app.sourceIFT, p.app.sourceClient, p.packet.Sequence)
	if err == nil {
		return fmt.Errorf(
			"e2etest: IFT packet %s pending transfer still present, want cleared",
			p.packet.reference(),
		)
	}
	if !isIFTPendingTransferNotFound(err) {
		return fmt.Errorf(
			"e2etest: IFT packet %s pending transfer lookup: want IFTPendingTransferNotFound, got: %w",
			p.packet.reference(),
			err,
		)
	}
	return nil
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

func (i *IFT) totalSupply(
	ctx context.Context,
	client *environment.EVM,
	contract common.Address,
) (*big.Int, error) {
	var supply *big.Int
	err := client.UseContractCaller(func(caller bind.ContractCaller) error {
		bound, err := ift.NewContractCaller(contract, caller)
		if err != nil {
			return fmt.Errorf("e2etest: bind IFT %s: %w", contract, err)
		}
		supply, err = bound.TotalSupply(&bind.CallOpts{Context: ctx})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("e2etest: query IFT total supply: %w", err)
	}
	return supply, nil
}

func (i *IFT) pendingTransfer(
	ctx context.Context,
	client *environment.EVM,
	contract common.Address,
	clientID string,
	sequence uint64,
) (ift.IIFTMsgsPendingTransfer, error) {
	var record ift.IIFTMsgsPendingTransfer
	err := client.UseContractCaller(func(caller bind.ContractCaller) error {
		bound, err := ift.NewContractCaller(contract, caller)
		if err != nil {
			return fmt.Errorf("e2etest: bind IFT %s: %w", contract, err)
		}
		record, err = bound.GetPendingTransfer(&bind.CallOpts{Context: ctx}, clientID, sequence)
		return err
	})
	if err != nil {
		return ift.IIFTMsgsPendingTransfer{}, err
	}
	return record, nil
}

func isIFTPendingTransferNotFound(err error) bool {
	revertData, ok := ethclient.RevertErrorData(err)
	if !ok || len(revertData) < 4 {
		return false
	}
	var selector [4]byte
	copy(selector[:], revertData)
	contractErr, lookupErr := iftABI.ErrorByID(selector)
	return lookupErr == nil && contractErr.Name == "IFTPendingTransferNotFound"
}
