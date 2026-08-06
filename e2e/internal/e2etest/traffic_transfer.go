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
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment/solidityibc/testerc20"
)

const (
	ics20DestPort         = "transfer"
	defaultTimeoutHorizon = 12 * time.Hour
	eventSendPacket       = "SendPacket"
	eventTimeoutPacket    = "TimeoutPacket"
)

var (
	ics20ABI = mustABI(ics20transfer.ContractMetaData)
	ics26ABI = mustABI(ics26router.ContractMetaData)
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
}

// Transfer binds ICS20 Transfer on a single directed route.
type Transfer struct {
	routeID      RouteID
	source       endpoint
	destination  endpoint
	sender       evm.Account
	sourceToken  common.Address
	sourceICS20  common.Address
	sourceRouter common.Address
	destICS20    common.Address
	sourceClient string
	destClient   string
}

type PreparedTransfer struct {
	app               *Transfer
	request           TransferRequest
	sourceBefore      *big.Int
	destinationBefore *big.Int
	timeoutTimestamp  uint64
	submitted         bool
}

type TransferPacket struct {
	app               *Transfer
	packet            Packet
	receiver          string
	amount            *big.Int
	sourceBefore      *big.Int
	destinationBefore *big.Int
}

var errTransferAlreadySubmitted = errors.New("e2etest: prepared Transfer has already been submitted")

func (i *Transfer) Send(ctx context.Context, request TransferRequest) (*TransferPacket, error) {
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
func (p *PreparedTransfer) Submit(ctx context.Context) (*TransferPacket, error) {
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
		SourceClient:     p.app.sourceClient,
		DestPort:         ics20DestPort,
		TimeoutTimestamp: p.timeoutTimestamp,
		Memo:             "",
	}
	data, err := ics20ABI.Pack("sendTransfer", msg)
	if err != nil {
		return nil, fmt.Errorf("e2etest: pack ICS20 sendTransfer: %w", err)
	}
	txHash, sequence, err := send(
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
	return &TransferPacket{
		app: p.app,
		packet: Packet{
			RouteID:      p.app.routeID,
			Source:       p.app.source.chain.ID(),
			SourceClient: p.app.sourceClient,
			SourceTxHash: txHash,
			Sequence:     sequence,
		},
		receiver:          p.request.Receiver,
		amount:            new(big.Int).Set(p.request.Amount),
		sourceBefore:      new(big.Int).Set(p.sourceBefore),
		destinationBefore: new(big.Int).Set(p.destinationBefore),
	}, nil
}

func (t *TransferPacket) Packet() Packet { return t.packet }

// VerifyDelivered waits for the destination voucher balance to reflect the
// submitted amount at the submitted receiver.
func (t *TransferPacket) VerifyDelivered(ctx context.Context) error {
	want := new(big.Int).Add(t.destinationBefore, t.amount)
	return awaitBalance(
		ctx,
		t.app.destination.chain,
		fmt.Sprintf("Transfer packet %s voucher delivery", t.packet.reference()),
		func(ctx context.Context) (*big.Int, error) {
			return t.app.voucherBalance(ctx, t.receiver)
		},
		want,
	)
}

func (t *TransferPacket) VerifyEscrowed(ctx context.Context) error {
	want := new(big.Int).Sub(t.sourceBefore, t.amount)
	got, err := erc20BalanceOf(ctx, t.app.source.evm, t.app.sourceToken, t.app.sender.Address())
	if err != nil {
		return err
	}
	if got.Cmp(want) != 0 {
		return fmt.Errorf(
			"e2etest: Transfer packet %s escrowed balance of %s: got %s, want %s",
			t.packet.reference(),
			t.app.sender.Address().Hex(),
			got,
			want,
		)
	}
	return nil
}

func (t *TransferPacket) VerifyNotMinted(ctx context.Context) error {
	got, err := t.app.voucherBalance(ctx, t.receiver)
	if err != nil {
		return err
	}
	if got.Cmp(t.destinationBefore) != 0 {
		return fmt.Errorf(
			"e2etest: Transfer packet %s not minted balance of %s: got %s, want %s",
			t.packet.reference(),
			t.receiver,
			got,
			t.destinationBefore,
		)
	}
	return nil
}

// VerifyRefunded waits for the source sender balance to be restored after timeout.
func (t *TransferPacket) VerifyRefunded(ctx context.Context) error {
	if err := awaitPacketTimeout(ctx, t.app.source, t.app.sourceRouter, t.app.sourceClient, t.packet); err != nil {
		return err
	}
	return awaitBalance(
		ctx,
		t.app.source.chain,
		fmt.Sprintf("Transfer packet %s refund", t.packet.reference()),
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
	return "transfer/" + i.destClient + "/" + strings.ToLower(i.sourceToken.Hex())
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

// awaitPacketTimeout waits for the source router to emit TimeoutPacket for the
// given packet.
func awaitPacketTimeout(
	ctx context.Context,
	source endpoint,
	router common.Address,
	sourceClient string,
	packet Packet,
) error {
	definition := ics26ABI.Events[eventTimeoutPacket]
	parser := mustBinding(ics26router.NewContractFilterer(router, nil))
	description := fmt.Sprintf("timeout for packet %s", packet.reference())
	_, err := awaitEvent(
		ctx,
		eventSource{endpoint: source, contract: router},
		definition.ID,
		description,
		func(log types.Log) (*ics26router.ContractTimeoutPacket, error) {
			event, err := parser.ParseTimeoutPacket(log)
			if err != nil {
				return nil, fmt.Errorf("e2etest: decode TimeoutPacket: %w", err)
			}
			return event, nil
		},
		func(event *ics26router.ContractTimeoutPacket) bool {
			return event.Packet.Sequence == packet.Sequence &&
				event.Packet.SourceClient == sourceClient
		},
	)
	return err
}

func sendPacketSequence(router common.Address) func(*types.Receipt) (uint64, bool, error) {
	parser := mustBinding(ics26router.NewContractFilterer(router, nil))
	return func(receipt *types.Receipt) (uint64, bool, error) {
		definition := ics26ABI.Events[eventSendPacket]
		for _, log := range receipt.Logs {
			if log.Address != router || len(log.Topics) == 0 || log.Topics[0] != definition.ID {
				continue
			}
			event, err := parser.ParseSendPacket(*log)
			if err != nil {
				return 0, false, fmt.Errorf("e2etest: decode SendPacket: %w", err)
			}
			return event.Packet.Sequence, true, nil
		}
		return 0, false, nil
	}
}

// sendPacketSequences returns every SendPacket sequence emitted by the router
// in one receipt, in log order
func sendPacketSequences(router common.Address, receipt *types.Receipt) ([]uint64, error) {
	parser := mustBinding(ics26router.NewContractFilterer(router, nil))
	definition := ics26ABI.Events[eventSendPacket]
	var sequences []uint64
	for _, log := range receipt.Logs {
		if log.Address != router || len(log.Topics) == 0 || log.Topics[0] != definition.ID {
			continue
		}
		event, err := parser.ParseSendPacket(*log)
		if err != nil {
			return nil, fmt.Errorf("e2etest: decode SendPacket: %w", err)
		}
		sequences = append(sequences, event.Packet.Sequence)
	}
	return sequences, nil
}
