// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc-contracts/packages/go-abigen/ics26router"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/gen/go/solidity-abi/testerc20"
)

func mustABI(metadata *bind.MetaData) abi.ABI {
	parsed, err := metadata.GetAbi()
	if err != nil {
		panic(fmt.Sprintf("e2etest: parse generated contract ABI: %v", err))
	}
	return *parsed
}

func mustBinding[T any](binding T, err error) T {
	if err != nil {
		panic(fmt.Sprintf("e2etest: construct generated contract binding: %v", err))
	}
	return binding
}

func calldata(call func(*bind.TransactOpts) (*types.Transaction, error)) ([]byte, error) {
	opts := &bind.TransactOpts{
		Nonce:    new(big.Int),
		Signer:   func(_ common.Address, tx *types.Transaction) (*types.Transaction, error) { return tx, nil },
		GasLimit: 1,
		GasPrice: big.NewInt(1),
		NoSend:   true,
	}
	tx, err := call(opts)
	if err != nil {
		return nil, err
	}
	return tx.Data(), nil
}

func receiptEvents[T any](
	receipt *types.Receipt,
	contract common.Address,
	parse func(types.Log) (*T, error),
) ([]*T, error) {
	var events []*T
	for _, log := range receipt.Logs {
		if log.Address != contract {
			continue
		}
		event, err := parse(*log)
		switch {
		case errors.Is(err, bind.ErrNoEventSignature), errors.Is(err, bind.ErrEventSignatureMismatch):
			continue
		case err != nil:
			return nil, err
		default:
			events = append(events, event)
		}
	}
	return events, nil
}

// SentPackets returns the packets the router sent over sourceClientID in one
// receipt, in log order.
func SentPackets(
	router common.Address,
	sourceClientID string,
	receipt *types.Receipt,
) ([]ics26router.IICS26RouterMsgsPacket, error) {
	parser := mustBinding(ics26router.NewContractFilterer(router, nil))
	events, err := receiptEvents(receipt, router, parser.ParseSendPacket)
	if err != nil {
		return nil, fmt.Errorf("e2etest: decode SendPacket: %w", err)
	}
	var packets []ics26router.IICS26RouterMsgsPacket
	for _, event := range events {
		if event.Packet.SourceClient == sourceClientID {
			packets = append(packets, event.Packet)
		}
	}
	return packets, nil
}

func transactionEventPacketIDs[T any](
	ctx context.Context,
	target endpoint,
	contract common.Address,
	txHash string,
	parse func(types.Log) (*T, error),
	packet func(*T) ics26router.IICS26RouterMsgsPacket,
) ([]PacketID, error) {
	receipt, err := target.evm.AwaitTransactionReceipt(ctx, common.HexToHash(txHash))
	if err != nil {
		return nil, fmt.Errorf("fetch transaction %s: %w", txHash, err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("transaction %s failed", txHash)
	}
	events, err := receiptEvents(receipt, contract, parse)
	if err != nil {
		return nil, err
	}
	ids := make([]PacketID, len(events))
	for i, event := range events {
		p := packet(event)
		ids[i] = PacketID{SourceClientID: p.SourceClient, Sequence: p.Sequence}
	}
	return ids, nil
}

// writeAcknowledgementPacketIDs returns the IDs of the packets a receive
// transaction wrote acknowledgements for.
func writeAcknowledgementPacketIDs(
	ctx context.Context,
	target endpoint,
	router common.Address,
	txHash string,
) ([]PacketID, error) {
	parser := mustBinding(ics26router.NewContractFilterer(router, nil))
	ids, err := transactionEventPacketIDs(
		ctx, target, router, txHash, parser.ParseWriteAcknowledgement,
		func(event *ics26router.ContractWriteAcknowledgement) ics26router.IICS26RouterMsgsPacket {
			return event.Packet
		},
	)
	if err != nil {
		return nil, fmt.Errorf("e2etest: read WriteAcknowledgement events: %w", err)
	}
	return ids, nil
}

// ackPacketIDs returns the IDs of the packets an acknowledgement transaction
// acknowledged.
func ackPacketIDs(ctx context.Context, target endpoint, router common.Address, txHash string) ([]PacketID, error) {
	parser := mustBinding(ics26router.NewContractFilterer(router, nil))
	ids, err := transactionEventPacketIDs(
		ctx, target, router, txHash, parser.ParseAckPacket,
		func(event *ics26router.ContractAckPacket) ics26router.IICS26RouterMsgsPacket { return event.Packet },
	)
	if err != nil {
		return nil, fmt.Errorf("e2etest: read AckPacket events: %w", err)
	}
	return ids, nil
}

// timeoutPacketIDs returns the IDs of the packets a timeout transaction timed
// out.
func timeoutPacketIDs(ctx context.Context, target endpoint, router common.Address, txHash string) ([]PacketID, error) {
	parser := mustBinding(ics26router.NewContractFilterer(router, nil))
	ids, err := transactionEventPacketIDs(
		ctx, target, router, txHash, parser.ParseTimeoutPacket,
		func(event *ics26router.ContractTimeoutPacket) ics26router.IICS26RouterMsgsPacket { return event.Packet },
	)
	if err != nil {
		return nil, fmt.Errorf("e2etest: read TimeoutPacket events: %w", err)
	}
	return ids, nil
}

// NewAddress generates a fresh, unfunded EVM address for use as a test
// recipient (e.g. an ERC20 transfer target).
func NewAddress() (common.Address, error) {
	account, err := evm.NewAccount()
	if err != nil {
		return common.Address{}, fmt.Errorf("e2etest: generate address: %w", err)
	}
	return account.Address(), nil
}

// erc20BalanceOf queries a TestERC20 contract's balance for holder.
func erc20BalanceOf(
	ctx context.Context,
	client *environment.EVM,
	contract, holder common.Address,
) (*big.Int, error) {
	var balance *big.Int
	err := client.UseContractCaller(func(caller bind.ContractCaller) error {
		bound, err := testerc20.NewTestERC20Caller(contract, caller)
		if err != nil {
			return fmt.Errorf("e2etest: bind TestERC20 %s: %w", contract, err)
		}
		balance, err = bound.BalanceOf(&bind.CallOpts{Context: ctx}, holder)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("e2etest: query TestERC20 %s balance of %s: %w", contract, holder, err)
	}
	return balance, nil
}

func deployContract(
	ctx context.Context,
	client *environment.EVM,
	sender evm.Account,
	metadata *bind.MetaData,
	constructorArgs ...any,
) (common.Address, error) {
	parsed := mustABI(metadata)
	payload := common.FromHex(metadata.Bin)
	if len(constructorArgs) > 0 {
		packed, err := parsed.Pack("", constructorArgs...)
		if err != nil {
			return common.Address{}, fmt.Errorf("e2etest: pack constructor: %w", err)
		}
		payload = append(payload, packed...)
	}
	receipt, err := client.BroadcastTx(ctx, sender, nil, payload, nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("e2etest: deploy contract: %w", err)
	}
	if receipt.ContractAddress == (common.Address{}) {
		return common.Address{}, errors.New("e2etest: deployment produced no contract address")
	}
	return receipt.ContractAddress, nil
}

// send broadcasts data to the source application and returns the packets the
// transaction sent over the route's client, in log order. It fails unless there
// are exactly want of them.
func (r sourceRoute) send(ctx context.Context, to common.Address, data []byte, want int) ([]sendResult, error) {
	receipt, err := r.source.evm.BroadcastTx(ctx, r.sender, &to, data, nil)
	if err != nil {
		return nil, fmt.Errorf("e2etest: broadcast transaction: %w", err)
	}
	packets, err := SentPackets(r.sourceRouter, r.sourceClientID, receipt)
	if err != nil {
		return nil, err
	}
	if len(packets) != want {
		return nil, fmt.Errorf(
			"e2etest: source transaction %s sent %d packets over client %q, want %d",
			receipt.TxHash.Hex(),
			len(packets),
			r.sourceClientID,
			want,
		)
	}
	sent := make([]sendResult, len(packets))
	for k, packet := range packets {
		sent[k] = sendResult{
			packetTx: PacketTx{
				RouteID:           r.routeID,
				Source:            r.source.chain.ID(),
				SourceClientID:    packet.SourceClient,
				SourceTxHash:      receipt.TxHash.Hex(),
				SourceBlockNumber: receipt.BlockNumber.Uint64(),
				Sequence:          packet.Sequence,
			},
			packet: packet,
		}
	}
	return sent, nil
}

func awaitBalance(
	ctx context.Context,
	chain *environment.Chain,
	description string,
	probe func(context.Context) (*big.Int, error),
	want *big.Int,
) error {
	timing := chain.Timing()
	_, err := await(
		ctx,
		timing.CompletionBudget,
		timing.PollInterval,
		description,
		func(ctx context.Context) (struct{}, bool, error) {
			got, err := probe(ctx)
			if err != nil {
				return struct{}{}, false, err
			}
			if got.Cmp(want) != 0 {
				return struct{}{}, false, fmt.Errorf("balance %s, want %s", got, want)
			}
			return struct{}{}, true, nil
		},
	)
	return err
}
