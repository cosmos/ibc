// SPDX-License-Identifier: Apache-2.0

package e2etest

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ibcerc20"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics20transfer"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	transfertypes "github.com/cosmos/ibc-go/v11/modules/apps/transfer/types"
	hostv2 "github.com/cosmos/ibc-go/v11/modules/core/24-host/v2"
	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/testerc20"
)

const (
	defaultTimeoutHorizon = 12 * time.Hour
)

var (
	ics20ABI = mustABI(ics20transfer.ContractMetaData)
	tokenABI = mustABI(testerc20.TestERC20MetaData)
)

type TransferRequest struct {
	// Amount must be positive and fit in a uint256. Prepare takes its own copy.
	Amount *big.Int
	// Receiver defaults to a fresh address on the destination Chain. Non-empty
	// values are forwarded to ICS20 as-is (including invalid hex for error-ack).
	Receiver string
	// Timeout is the destination-relative packet lifetime. Non-positive selects
	// a far-future-but-valid default; positive values are rounded up to whole seconds.
	Timeout time.Duration
	// Memo is forwarded to ICS20 as-is.
	Memo string
}

// Transfer drives ICS20 Transfer on a single directed route.
type Transfer struct {
	routeID        RouteID
	source         endpoint
	destination    endpoint
	sender         evm.Account
	sourceToken    common.Address
	sourceICS20    common.Address
	sourceRouter   common.Address
	destRouter     common.Address
	destICS20      common.Address
	sourceClientID string
	destClientID   string
}

type PreparedTransfer struct {
	app               *Transfer
	request           TransferRequest
	sourceBefore      *big.Int
	destinationBefore *big.Int
	timeoutTimestamp  uint64
	submitted         bool
}

// TransferSend is the result of a Transfer Send or Submit: the packet it
// emitted plus the balance baselines its Verify methods compare against.
type TransferSend struct {
	sendResult
	app               *Transfer
	receiver          string
	amount            *big.Int
	sourceBefore      *big.Int
	destinationBefore *big.Int
}

var errTransferAlreadySubmitted = errors.New("e2etest: prepared Transfer has already been submitted")

func (i *Transfer) Send(ctx context.Context, request TransferRequest) (*TransferSend, error) {
	prepared, err := i.Prepare(ctx, request)
	if err != nil {
		return nil, err
	}
	return prepared.Submit(ctx)
}

func (i *Transfer) Prepare(ctx context.Context, request TransferRequest) (*PreparedTransfer, error) {
	amount, err := validAmount(request.Amount)
	if err != nil {
		return nil, err
	}
	receiver, err := i.receiver(request.Receiver)
	if err != nil {
		return nil, err
	}
	sourceBefore, err := erc20BalanceOf(ctx, i.source.evm, i.sourceToken, i.sender.Address())
	if err != nil {
		return nil, err
	}
	destinationBefore, err := i.voucherBalance(ctx, receiver)
	if err != nil {
		return nil, err
	}
	timeoutTimestamp, err := destinationTimeout(ctx, i.destination, request.Timeout)
	if err != nil {
		return nil, err
	}
	request.Amount = amount
	request.Receiver = receiver
	return &PreparedTransfer{
		app:               i,
		request:           request,
		sourceBefore:      sourceBefore,
		destinationBefore: destinationBefore,
		timeoutTimestamp:  timeoutTimestamp,
	}, nil
}

// Submit consumes the prepared transfer before broadcasting. A failed attempt
// cannot be retried because its balance baselines may no longer be current.
func (p *PreparedTransfer) Submit(ctx context.Context) (*TransferSend, error) {
	if p.submitted {
		return nil, errTransferAlreadySubmitted
	}
	p.submitted = true

	approve, err := tokenABI.Pack("approve", p.app.sourceICS20, p.request.Amount)
	if err != nil {
		return nil, fmt.Errorf("e2etest: pack ERC20 approve: %w", err)
	}
	if _, approveErr := p.app.source.evm.BroadcastTx(
		ctx,
		p.app.sender,
		&p.app.sourceToken,
		approve,
		nil,
	); approveErr != nil {
		return nil, fmt.Errorf("e2etest: approve ICS20 spender on route %q: %w", p.app.routeID, approveErr)
	}

	msg := ics20transfer.IICS20TransferMsgsSendTransferMsg{
		Denom:            p.app.sourceToken,
		Amount:           p.request.Amount,
		Receiver:         p.request.Receiver,
		SourceClient:     p.app.sourceClientID,
		DestPort:         transfertypes.PortID,
		TimeoutTimestamp: p.timeoutTimestamp,
		Memo:             p.request.Memo,
	}
	data, err := ics20ABI.Pack("sendTransfer", msg)
	if err != nil {
		return nil, fmt.Errorf("e2etest: pack ICS20 sendTransfer: %w", err)
	}
	receipt, sequence, err := send(
		ctx,
		p.app.source.evm,
		p.app.sender,
		p.app.sourceICS20,
		data,
		sendPacketSequence(p.app.sourceRouter),
	)
	if err != nil {
		return nil, fmt.Errorf("e2etest: send Transfer on route %q: %w", p.app.routeID, err)
	}
	return &TransferSend{
		sendResult: newSendResult(
			p.app.routeID,
			p.app.source,
			p.app.sourceClientID,
			receipt,
			sequence,
		),
		app:               p.app,
		receiver:          p.request.Receiver,
		amount:            new(big.Int).Set(p.request.Amount),
		sourceBefore:      new(big.Int).Set(p.sourceBefore),
		destinationBefore: new(big.Int).Set(p.destinationBefore),
	}, nil
}

// VerifyCommitmentCreated checks the source commitment at the send transaction's block.
func (t *TransferSend) VerifyCommitmentCreated(ctx context.Context) error {
	commitment, err := getCommitment(
		ctx,
		t.app.source,
		t.app.sourceRouter,
		crypto.Keccak256Hash(hostv2.PacketCommitmentKey(t.app.sourceClientID, t.packetTx.Sequence)),
		new(big.Int).SetUint64(t.packetTx.SourceBlockNumber),
	)
	if err != nil {
		return fmt.Errorf("e2etest: query Transfer packet %s source commitment: %w", t.packetTx.reference(), err)
	}
	if commitment == ([32]byte{}) {
		return fmt.Errorf("e2etest: Transfer packet %s source commitment was not created", t.packetTx.reference())
	}
	return nil
}

// VerifyReceiptCreated checks that the destination recorded the packet receipt.
func (t *TransferSend) VerifyReceiptCreated(ctx context.Context) error {
	receipt, err := getCommitment(
		ctx,
		t.app.destination,
		t.app.destRouter,
		crypto.Keccak256Hash(hostv2.PacketReceiptKey(t.app.destClientID, t.packetTx.Sequence)),
		nil,
	)
	if err != nil {
		return fmt.Errorf("e2etest: query Transfer packet %s destination receipt: %w", t.packetTx.reference(), err)
	}
	if receipt == ([32]byte{}) {
		return fmt.Errorf("e2etest: Transfer packet %s destination receipt was not created", t.packetTx.reference())
	}
	return nil
}

// VerifyCommitmentCleared checks that acknowledgement or timeout removed the source commitment.
func (t *TransferSend) VerifyCommitmentCleared(ctx context.Context) error {
	commitment, err := getCommitment(
		ctx,
		t.app.source,
		t.app.sourceRouter,
		crypto.Keccak256Hash(hostv2.PacketCommitmentKey(t.app.sourceClientID, t.packetTx.Sequence)),
		nil,
	)
	if err != nil {
		return fmt.Errorf("e2etest: query Transfer packet %s source commitment: %w", t.packetTx.reference(), err)
	}
	if commitment != ([32]byte{}) {
		return fmt.Errorf("e2etest: Transfer packet %s source commitment was not cleared", t.packetTx.reference())
	}
	return nil
}

func (t *TransferSend) successfulReceipt(
	ctx context.Context,
	target endpoint,
	action string,
	txHash string,
) (*types.Receipt, error) {
	receipt, err := target.evm.AwaitTransactionReceipt(ctx, common.HexToHash(txHash))
	if err != nil {
		return nil, fmt.Errorf(
			"e2etest: fetch Transfer packet %s %s transaction %s: %w",
			t.packetTx.reference(), action, txHash, err,
		)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf(
			"e2etest: Transfer packet %s %s transaction %s failed",
			t.packetTx.reference(), action, txHash,
		)
	}
	return receipt, nil
}

// VerifyAcknowledgementWritten checks that txHash succeeded with one WriteAcknowledgement for this packet.
func (t *TransferSend) VerifyAcknowledgementWritten(ctx context.Context, txHash string) error {
	receipt, err := t.successfulReceipt(ctx, t.app.destination, "receive", txHash)
	if err != nil {
		return err
	}

	parser := mustBinding(ics26router.NewContractFilterer(t.app.destRouter, nil))
	events, err := receiptEvents(receipt, t.app.destRouter, parser.ParseWriteAcknowledgement)
	if err != nil {
		return fmt.Errorf("e2etest: decode WriteAcknowledgement: %w", err)
	}
	matches := 0
	for _, event := range events {
		if event.Packet.SourceClient == t.app.sourceClient && event.Packet.Sequence == t.packetTx.Sequence {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf(
			"e2etest: Transfer packet %s receive transaction %s emitted %d matching "+
				"WriteAcknowledgement events, want 1",
			t.packetTx.reference(), txHash, matches,
		)
	}
	return nil
}

// VerifyAcknowledgementExecuted checks that txHash succeeded with one AckPacket for this packet.
func (t *TransferSend) VerifyAcknowledgementExecuted(ctx context.Context, txHash string) error {
	receipt, err := t.successfulReceipt(ctx, t.app.source, "acknowledgement", txHash)
	if err != nil {
		return err
	}

	parser := mustBinding(ics26router.NewContractFilterer(t.app.sourceRouter, nil))
	events, err := receiptEvents(receipt, t.app.sourceRouter, parser.ParseAckPacket)
	if err != nil {
		return fmt.Errorf("e2etest: decode AckPacket: %w", err)
	}
	matches := 0
	for _, event := range events {
		if event.Packet.SourceClient == t.app.sourceClientID && event.Packet.Sequence == t.packetTx.Sequence {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf(
			"e2etest: Transfer packet %s acknowledgement transaction %s emitted %d matching AckPacket events, want 1",
			t.packetTx.reference(), txHash, matches,
		)
	}
	return nil
}

func getCommitment(
	ctx context.Context,
	endpoint endpoint,
	router common.Address,
	path common.Hash,
	blockNumber *big.Int,
) ([32]byte, error) {
	var commitment [32]byte
	err := endpoint.evm.UseContractCaller(func(caller bind.ContractCaller) error {
		bound, err := ics26router.NewContractCaller(router, caller)
		if err != nil {
			return fmt.Errorf("bind ICS26Router %s: %w", router, err)
		}
		commitment, err = bound.GetCommitment(&bind.CallOpts{Context: ctx, BlockNumber: blockNumber}, path)
		return err
	})
	return commitment, err
}

// VerifyDelivered waits for the destination voucher balance to reflect the
// submitted amount at the submitted receiver.
func (t *TransferSend) VerifyDelivered(ctx context.Context) error {
	want := new(big.Int).Add(t.destinationBefore, t.amount)
	return awaitBalance(
		ctx,
		t.app.destination.chain,
		fmt.Sprintf("Transfer packet %s voucher delivery", t.packetTx.reference()),
		func(ctx context.Context) (*big.Int, error) {
			return t.app.voucherBalance(ctx, t.receiver)
		},
		want,
	)
}

func (t *TransferSend) VerifyEscrowed(ctx context.Context) error {
	want := new(big.Int).Sub(t.sourceBefore, t.amount)
	got, err := erc20BalanceOf(ctx, t.app.source.evm, t.app.sourceToken, t.app.sender.Address())
	if err != nil {
		return err
	}
	if got.Cmp(want) != 0 {
		return fmt.Errorf(
			"e2etest: Transfer packet %s escrowed balance of %s: got %s, want %s",
			t.packetTx.reference(),
			t.app.sender.Address().Hex(),
			got,
			want,
		)
	}
	return nil
}

func (t *TransferSend) VerifyNotMinted(ctx context.Context) error {
	got, err := t.app.voucherBalance(ctx, t.receiver)
	if err != nil {
		return err
	}
	if got.Cmp(t.destinationBefore) != 0 {
		return fmt.Errorf(
			"e2etest: Transfer packet %s not minted balance of %s: got %s, want %s",
			t.packetTx.reference(),
			t.receiver,
			got,
			t.destinationBefore,
		)
	}
	return nil
}

// VerifyRefunded waits for the source sender balance to be restored after timeout.
func (t *TransferSend) VerifyRefunded(ctx context.Context, txHash string) error {
	if err := verifyPacketTimeout(
		ctx, t.app.source, t.app.sourceRouter, t.app.sourceClientID, t.packetTx, txHash,
	); err != nil {
		return err
	}
	return awaitBalance(
		ctx,
		t.app.source.chain,
		fmt.Sprintf("Transfer packet %s refund", t.packetTx.reference()),
		func(ctx context.Context) (*big.Int, error) {
			return erc20BalanceOf(ctx, t.app.source.evm, t.app.sourceToken, t.app.sender.Address())
		},
		t.sourceBefore,
	)
}

func validAmount(amount *big.Int) (*big.Int, error) {
	if amount == nil {
		return nil, errors.New("e2etest: Transfer amount is required")
	}
	owned := new(big.Int).Set(amount)
	if owned.Sign() <= 0 {
		return nil, fmt.Errorf("e2etest: Transfer amount must be positive, got %s", owned)
	}
	if owned.BitLen() > 256 {
		return nil, fmt.Errorf("e2etest: Transfer amount %s exceeds uint256", owned)
	}
	return owned, nil
}

func (i *Transfer) receiver(value string) (string, error) {
	if value != "" {
		return value, nil
	}
	account, err := evm.NewAccount()
	if err != nil {
		return "", fmt.Errorf("e2etest: generate Transfer receiver: %w", err)
	}
	return account.Address().Hex(), nil
}

// destinationTimeout derives a packet timeout timestamp from the destination
// Chain head, since the destination clock decides when packets expire.
func destinationTimeout(
	ctx context.Context,
	destination endpoint,
	timeout time.Duration,
) (uint64, error) {
	header, err := destination.evm.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf(
			"e2etest: read destination Chain %q head for packet timeout: %w",
			destination.chain.ID(),
			err,
		)
	}
	horizon := timeout
	if horizon <= 0 {
		horizon = defaultTimeoutHorizon
	}
	seconds := uint64((horizon + time.Second - 1) / time.Second)
	return header.Time + seconds, nil
}

func (i *Transfer) voucherDenom() string {
	return transfertypes.NewDenom(
		strings.ToLower(i.sourceToken.Hex()),
		transfertypes.NewHop(transfertypes.PortID, i.destClientID),
	).Path()
}

func (i *Transfer) voucherBalance(ctx context.Context, holder string) (*big.Int, error) {
	if !common.IsHexAddress(holder) {
		return big.NewInt(0), nil
	}
	holderAddr := common.HexToAddress(holder)
	balance := big.NewInt(0)
	err := i.destination.evm.UseContractCaller(func(caller bind.ContractCaller) error {
		bound, err := ics20transfer.NewContractCaller(i.destICS20, caller)
		if err != nil {
			return fmt.Errorf("e2etest: bind destination ICS20 %s: %w", i.destICS20, err)
		}
		voucher, err := bound.IbcERC20Contract(&bind.CallOpts{Context: ctx}, i.voucherDenom())
		if err != nil {
			if isICS20DenomNotFound(err) {
				return nil
			}
			return fmt.Errorf("e2etest: query destination ICS20 voucher: %w", err)
		}
		token, err := ibcerc20.NewContractCaller(voucher, caller)
		if err != nil {
			return fmt.Errorf("e2etest: bind IBCERC20 %s: %w", voucher, err)
		}
		got, err := token.BalanceOf(&bind.CallOpts{Context: ctx}, holderAddr)
		if err != nil {
			return fmt.Errorf("e2etest: query IBCERC20 %s balance: %w", voucher, err)
		}
		balance = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return balance, nil
}

func isICS20DenomNotFound(err error) bool {
	revertData, ok := ethclient.RevertErrorData(err)
	if !ok || len(revertData) < 4 {
		return false
	}
	var selector [4]byte
	copy(selector[:], revertData)
	contractErr, lookupErr := ics20ABI.ErrorByID(selector)
	return lookupErr == nil && contractErr.Name == "ICS20DenomNotFound"
}

// verifyPacketTimeout checks that txHash succeeded with one TimeoutPacket for the packet.
func verifyPacketTimeout(
	ctx context.Context,
	source endpoint,
	router common.Address,
	sourceClientID string,
	packetTx PacketTx,
	txHash string,
) error {
	receipt, err := source.evm.AwaitTransactionReceipt(ctx, common.HexToHash(txHash))
	if err != nil {
		return fmt.Errorf("e2etest: fetch packet %s timeout transaction %s: %w", packetTx.reference(), txHash, err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("e2etest: packet %s timeout transaction %s failed", packetTx.reference(), txHash)
	}

	parser := mustBinding(ics26router.NewContractFilterer(router, nil))
	events, err := receiptEvents(receipt, router, parser.ParseTimeoutPacket)
	if err != nil {
		return fmt.Errorf("e2etest: decode TimeoutPacket: %w", err)
	}
	matches := 0
	for _, event := range events {
		if event.Packet.SourceClient == sourceClientID && event.Packet.Sequence == packetTx.Sequence {
			matches++
		}
	}
	if matches != 1 {
		return fmt.Errorf(
			"e2etest: packet %s timeout transaction %s emitted %d matching TimeoutPacket events, want 1",
			packetTx.reference(), txHash, matches,
		)
	}
	return nil
}

func sendPacketSequence(router common.Address) func(*types.Receipt) (uint64, bool, error) {
	return func(receipt *types.Receipt) (uint64, bool, error) {
		sequences, err := sendPacketSequences(router, receipt)
		if err != nil {
			return 0, false, err
		}
		if len(sequences) == 0 {
			return 0, false, nil
		}
		return sequences[0], true, nil
	}
}

// sendPacketSequences returns every SendPacket sequence emitted by the router
// in one receipt, in log order
func sendPacketSequences(router common.Address, receipt *types.Receipt) ([]uint64, error) {
	parser := mustBinding(ics26router.NewContractFilterer(router, nil))
	events, err := receiptEvents(receipt, router, parser.ParseSendPacket)
	if err != nil {
		return nil, fmt.Errorf("e2etest: decode SendPacket: %w", err)
	}
	sequences := make([]uint64, len(events))
	for i, event := range events {
		sequences[i] = event.Packet.Sequence
	}
	return sequences, nil
}
