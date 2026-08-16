// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"context"
	"fmt"
	"math/big"
	"slices"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ift"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/solidity-abi/iftbatchtransfershim"
)

var (
	iftABI                 = mustABI(ift.ContractMetaData)
	iftTransactor          = mustBinding(ift.NewContractTransactor(common.Address{}, nil))
	iftBatchShimTransactor = mustBinding(iftbatchtransfershim.NewIFTBatchTransferShimTransactor(common.Address{}, nil))
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

// IFT drives the Interchain Fungible Token pair on a single directed route.
type IFT struct {
	routeID        RouteID
	source         endpoint
	destination    endpoint
	sender         evm.Account
	sourceIFT      common.Address
	destIFT        common.Address
	sourceRouter   common.Address
	destRouter     common.Address
	sourceClientID string
	batcher        common.Address
}

// IFTSend is the result of an IFT Send: the packet it emitted plus the balance
// baselines its Verify methods compare against.
type IFTSend struct {
	sendResult
	app                     *IFT
	receiver                common.Address
	amount                  *big.Int
	sourceBefore            *big.Int
	destinationBefore       *big.Int
	destinationSupplyBefore *big.Int
	timeoutTimestamp        uint64
}

func (i *IFT) Send(ctx context.Context, request IFTRequest) (*IFTSend, error) {
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
	data, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return iftTransactor.IftTransfer(opts, i.sourceClientID, receiver.Hex(), amount, timeoutTimestamp)
	})
	if err != nil {
		return nil, fmt.Errorf("e2etest: pack IFT iftTransfer: %w", err)
	}
	receipt, sequence, err := send(
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
	return &IFTSend{
		sendResult:              newSendResult(i.routeID, i.source, i.sourceClientID, receipt, sequence),
		app:                     i,
		receiver:                receiver,
		amount:                  amount,
		sourceBefore:            sourceBefore,
		destinationBefore:       destinationBefore,
		destinationSupplyBefore: destinationSupplyBefore,
		timeoutTimestamp:        timeoutTimestamp,
	}, nil
}

// SendBatch emits multiple IFT transfers from a single source transaction
func (i *IFT) SendBatch(ctx context.Context, requests []IFTRequest) (*IFTBatch, error) {
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

	batcherBefore, err := i.balance(ctx, i.source.evm, i.sourceIFT, i.batcher)
	if err != nil {
		return nil, err
	}
	destinationSupplyBefore, err := i.totalSupply(ctx, i.destination.evm, i.destIFT)
	if err != nil {
		return nil, err
	}
	data, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return iftBatchShimTransactor.BatchIftTransfer(opts, i.sourceIFT, i.sourceClientID, transfers)
	})
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

	packets := make([]iftBatchPacket, len(requests))
	for k, sequence := range sequences {
		packets[k] = iftBatchPacket{
			packetTx:          newSendResult(i.routeID, i.source, i.sourceClientID, receipt, sequence).PacketTx(),
			receiver:          common.HexToAddress(transfers[k].Receiver),
			amount:            transfers[k].Amount,
			destinationBefore: destinationsBefore[k],
			timeoutTimestamp:  transfers[k].TimeoutTimestamp,
		}
	}
	return &IFTBatch{
		app:                     i,
		packets:                 packets,
		receipt:                 receipt,
		batcherBefore:           batcherBefore,
		destinationSupplyBefore: destinationSupplyBefore,
		total:                   total,
	}, nil
}

type iftBatchPacket struct {
	packetTx          PacketTx
	receiver          common.Address
	amount            *big.Int
	destinationBefore *big.Int
	timeoutTimestamp  uint64
}

// IFTBatch is the result of a SendBatch call: several IFT packets emitted
// from a single source transaction.
type IFTBatch struct {
	app                     *IFT
	packets                 []iftBatchPacket
	receipt                 *types.Receipt
	batcherBefore           *big.Int
	destinationSupplyBefore *big.Int
	total                   *big.Int
}

// PacketTxs locates every packet the batch emitted, in send order. They all
// share the batch's single source transaction.
func (b *IFTBatch) PacketTxs() []PacketTx {
	packetTxs := make([]PacketTx, len(b.packets))
	for k, packet := range b.packets {
		packetTxs[k] = packet.packetTx
	}
	return packetTxs
}

// TxHash is the hex hash of the one source transaction that emitted the batch.
func (b *IFTBatch) TxHash() string { return b.receipt.TxHash.Hex() }

// Receipt is the batch's source transaction receipt.
func (b *IFTBatch) Receipt() *types.Receipt { return b.receipt }

// LatestTimeoutTimestamp returns the latest packet timeout in the batch.
func (b *IFTBatch) LatestTimeoutTimestamp() uint64 {
	var latest uint64
	for _, packet := range b.packets {
		latest = max(latest, packet.timeoutTimestamp)
	}
	return latest
}

func (b *IFTBatch) VerifyDelivered(ctx context.Context) error {
	for _, packet := range b.packets {
		want := new(big.Int).Add(packet.destinationBefore, packet.amount)
		err := awaitBalance(
			ctx,
			b.app.destination.chain,
			fmt.Sprintf("IFT packet %s mint delivery", packet.packetTx.reference()),
			func(ctx context.Context) (*big.Int, error) {
				return b.app.balance(ctx, b.app.destination.evm, b.app.destIFT, packet.receiver)
			},
			want,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// VerifyState checks exact source refunds, destination mints and pending
// records. delivered and refunded contain packet indexes.
func (b *IFTBatch) VerifyState(ctx context.Context, delivered, refunded []int) error {
	deliveredTotal := new(big.Int)
	for _, index := range delivered {
		if slices.Contains(refunded, index) {
			return fmt.Errorf("e2etest: IFT batch packet index %d is both delivered and refunded", index)
		}
		deliveredTotal.Add(deliveredTotal, b.packets[index].amount)
	}
	refundedTotal := new(big.Int)
	for _, index := range refunded {
		refundedTotal.Add(refundedTotal, b.packets[index].amount)
	}

	wantSource := new(big.Int).Sub(new(big.Int).Set(b.batcherBefore), b.total)
	wantSource.Add(wantSource, refundedTotal)
	gotSource, err := b.app.balance(ctx, b.app.source.evm, b.app.sourceIFT, b.app.batcher)
	if err != nil {
		return err
	}
	if gotSource.Cmp(wantSource) != 0 {
		return fmt.Errorf("e2etest: IFT batch source balance: got %s, want %s", gotSource, wantSource)
	}

	for index, packet := range b.packets {
		want := new(big.Int).Set(packet.destinationBefore)
		if slices.Contains(delivered, index) {
			want.Add(want, packet.amount)
		}
		got, balanceErr := b.app.balance(ctx, b.app.destination.evm, b.app.destIFT, packet.receiver)
		if balanceErr != nil {
			return balanceErr
		}
		if got.Cmp(want) != 0 {
			return fmt.Errorf(
				"e2etest: IFT batch destination balance for packet %d: got %s, want %s",
				index,
				got,
				want,
			)
		}

		isDelivered := slices.Contains(delivered, index)
		isRefunded := slices.Contains(refunded, index)
		record, pendingErr := b.app.pendingTransfer(
			ctx,
			b.app.source.evm,
			b.app.sourceIFT,
			b.app.sourceClientID,
			packet.packetTx.Sequence,
		)
		if isDelivered || isRefunded {
			if pendingErr == nil {
				return fmt.Errorf(
					"e2etest: IFT batch packet %d pending transfer still present, want cleared",
					index,
				)
			}
			if !isIFTPendingTransferNotFound(pendingErr) {
				return fmt.Errorf("e2etest: IFT batch packet %d pending transfer: %w", index, pendingErr)
			}
			continue
		}
		if pendingErr != nil {
			return fmt.Errorf("e2etest: IFT batch packet %d pending transfer: %w", index, pendingErr)
		}
		if record.Sender != b.app.batcher || record.Amount.Cmp(packet.amount) != 0 {
			return fmt.Errorf(
				"e2etest: IFT batch packet %d pending transfer: got sender %s amount %s, want sender %s amount %s",
				index,
				record.Sender.Hex(),
				record.Amount,
				b.app.batcher.Hex(),
				packet.amount,
			)
		}
	}

	wantSupply := new(big.Int).Add(new(big.Int).Set(b.destinationSupplyBefore), deliveredTotal)
	gotSupply, err := b.app.totalSupply(ctx, b.app.destination.evm, b.app.destIFT)
	if err != nil {
		return err
	}
	if gotSupply.Cmp(wantSupply) != 0 {
		return fmt.Errorf("e2etest: IFT batch destination supply: got %s, want %s", gotSupply, wantSupply)
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

func (p *IFTSend) TimeoutTimestamp() uint64 { return p.timeoutTimestamp }

// WriteAcknowledgementSequences returns the packet sequences written by a destination receive transaction.
func (i *IFT) WriteAcknowledgementSequences(ctx context.Context, txHash string) ([]uint64, error) {
	parser := mustBinding(ics26router.NewContractFilterer(i.destRouter, nil))
	sequences, err := transactionEventSequences(
		ctx,
		i.destination,
		i.destRouter,
		txHash,
		parser.ParseWriteAcknowledgement,
		func(event *ics26router.ContractWriteAcknowledgement) uint64 { return event.Packet.Sequence },
	)
	if err != nil {
		return nil, fmt.Errorf("e2etest: read IFT WriteAcknowledgement events: %w", err)
	}
	return sequences, nil
}

// AckPacketSequences returns the packet sequences acknowledged by a source acknowledgement transaction.
func (i *IFT) AckPacketSequences(ctx context.Context, txHash string) ([]uint64, error) {
	parser := mustBinding(ics26router.NewContractFilterer(i.sourceRouter, nil))
	sequences, err := transactionEventSequences(
		ctx,
		i.source,
		i.sourceRouter,
		txHash,
		parser.ParseAckPacket,
		func(event *ics26router.ContractAckPacket) uint64 { return event.Packet.Sequence },
	)
	if err != nil {
		return nil, fmt.Errorf("e2etest: read IFT AckPacket events: %w", err)
	}
	return sequences, nil
}

// TimeoutPacketSequences returns the packet sequences timed out by a source timeout transaction.
func (i *IFT) TimeoutPacketSequences(ctx context.Context, txHash string) ([]uint64, error) {
	parser := mustBinding(ics26router.NewContractFilterer(i.sourceRouter, nil))
	sequences, err := transactionEventSequences(
		ctx,
		i.source,
		i.sourceRouter,
		txHash,
		parser.ParseTimeoutPacket,
		func(event *ics26router.ContractTimeoutPacket) uint64 { return event.Packet.Sequence },
	)
	if err != nil {
		return nil, fmt.Errorf("e2etest: read IFT TimeoutPacket events: %w", err)
	}
	return sequences, nil
}

// VerifyBurned checks that the submitted amount left the source sender balance.
func (p *IFTSend) VerifyBurned(ctx context.Context) error {
	want := new(big.Int).Sub(p.sourceBefore, p.amount)
	got, err := p.app.balance(ctx, p.app.source.evm, p.app.sourceIFT, p.app.sender.Address())
	if err != nil {
		return err
	}
	if got.Cmp(want) != 0 {
		return fmt.Errorf(
			"e2etest: IFT packet %s burned balance of %s: got %s, want %s",
			p.packetTx.reference(),
			p.app.sender.Address().Hex(),
			got,
			want,
		)
	}
	return nil
}

// VerifyDelivered waits for the destination IFT to mint the submitted amount
// at the submitted receiver.
func (p *IFTSend) VerifyDelivered(ctx context.Context) error {
	want := new(big.Int).Add(p.destinationBefore, p.amount)
	return awaitBalance(
		ctx,
		p.app.destination.chain,
		fmt.Sprintf("IFT packet %s mint delivery", p.packetTx.reference()),
		func(ctx context.Context) (*big.Int, error) {
			return p.app.balance(ctx, p.app.destination.evm, p.app.destIFT, p.receiver)
		},
		want,
	)
}

func (p *IFTSend) VerifyNotMinted(ctx context.Context) error {
	got, err := p.app.balance(ctx, p.app.destination.evm, p.app.destIFT, p.receiver)
	if err != nil {
		return err
	}
	if got.Cmp(p.destinationBefore) != 0 {
		return fmt.Errorf(
			"e2etest: IFT packet %s not minted balance of %s: got %s, want %s",
			p.packetTx.reference(),
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
			p.packetTx.reference(),
			supply,
			p.destinationSupplyBefore,
		)
	}
	return nil
}

// VerifyRefunded waits for the source sender balance to be restored after the transfer completes.
func (p *IFTSend) VerifyRefunded(ctx context.Context) error {
	if err := p.awaitPendingCleared(ctx); err != nil {
		return err
	}
	return p.awaitSourceRefund(ctx)
}

func (p *IFTSend) awaitPendingCleared(ctx context.Context) error {
	timing := p.app.source.chain.Timing()
	_, err := await(
		ctx,
		timing.CompletionBudget,
		timing.PollInterval,
		fmt.Sprintf("IFT packet %s pending transfer cleared", p.packetTx.reference()),
		func(ctx context.Context) (struct{}, bool, error) {
			_, err := p.app.pendingTransfer(
				ctx,
				p.app.source.evm,
				p.app.sourceIFT,
				p.app.sourceClientID,
				p.packetTx.Sequence,
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

func (p *IFTSend) awaitSourceRefund(ctx context.Context) error {
	return awaitBalance(
		ctx,
		p.app.source.chain,
		fmt.Sprintf("IFT packet %s refund", p.packetTx.reference()),
		func(ctx context.Context) (*big.Int, error) {
			return p.app.balance(ctx, p.app.source.evm, p.app.sourceIFT, p.app.sender.Address())
		},
		p.sourceBefore,
	)
}

// VerifyPending checks that a pending-transfer record exists on the source
// IFT contract for this packet's sequence, with the expected sender and amount.
func (p *IFTSend) VerifyPending(ctx context.Context) error {
	record, err := p.app.pendingTransfer(
		ctx,
		p.app.source.evm,
		p.app.sourceIFT,
		p.app.sourceClientID,
		p.packetTx.Sequence,
	)
	if err != nil {
		return fmt.Errorf("e2etest: IFT packet %s pending transfer: %w", p.packetTx.reference(), err)
	}
	if record.Sender != p.app.sender.Address() {
		return fmt.Errorf(
			"e2etest: IFT packet %s pending transfer sender: got %s, want %s",
			p.packetTx.reference(),
			record.Sender.Hex(),
			p.app.sender.Address().Hex(),
		)
	}
	if record.Amount.Cmp(p.amount) != 0 {
		return fmt.Errorf(
			"e2etest: IFT packet %s pending transfer amount: got %s, want %s",
			p.packetTx.reference(),
			record.Amount,
			p.amount,
		)
	}
	return nil
}

// VerifyPendingCleared checks that no pending-transfer record exists for
// this packet's sequence on the source IFT contract.
func (p *IFTSend) VerifyPendingCleared(ctx context.Context) error {
	_, err := p.app.pendingTransfer(ctx, p.app.source.evm, p.app.sourceIFT, p.app.sourceClientID, p.packetTx.Sequence)
	if err == nil {
		return fmt.Errorf(
			"e2etest: IFT packet %s pending transfer still present, want cleared",
			p.packetTx.reference(),
		)
	}
	if !isIFTPendingTransferNotFound(err) {
		return fmt.Errorf(
			"e2etest: IFT packet %s pending transfer lookup: want IFTPendingTransferNotFound, got: %w",
			p.packetTx.reference(),
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
