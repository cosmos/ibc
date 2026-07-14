package testapp

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/testapp/contracts"
)

const (
	eventIFTSent     = "IFTSent"
	eventIFTReceived = "IFTReceived"
	eventIFTRefunded = "IFTRefunded"
)

var iftABI = contracts.MockIFT.MustABI()

type IFTRequest struct {
	// Amount must be positive and fit in a uint256. Prepare takes its own copy.
	Amount *big.Int
	// Receiver defaults to a fresh address on the destination Chain.
	Receiver string
	// Timeout is disabled when non-positive and rounded up to whole seconds otherwise.
	Timeout time.Duration
}

// IFT binds MockIFT on a single directed route.
type IFT struct {
	routeID     RouteID
	source      endpoint
	destination endpoint
	sender      evm.Account
	sourceApp   common.Address
	destApp     common.Address
}

func NewIFT(
	routeID RouteID,
	source, destination *environment.Chain,
	sender evm.Account,
	contracts IFTContracts,
) (*IFT, error) {
	sourceEndpoint, destinationEndpoint, err := bindRoute(routeID, source, destination)
	if err != nil {
		return nil, err
	}
	sourceApp, err := address("source MockIFT", contracts.Source)
	if err != nil {
		return nil, err
	}
	destinationApp, err := address("destination MockIFT", contracts.Destination)
	if err != nil {
		return nil, err
	}
	return &IFT{
		routeID:     routeID,
		source:      sourceEndpoint,
		destination: destinationEndpoint,
		sender:      sender,
		sourceApp:   sourceApp,
		destApp:     destinationApp,
	}, nil
}

type PreparedIFT struct {
	app               *IFT
	request           IFTRequest
	sourceBefore      *big.Int
	destinationBefore *big.Int
	timeoutTimestamp  uint64
	submitted         bool
}

type IFTTransfer struct {
	app               *IFT
	packet            Packet
	receiver          common.Address
	amount            *big.Int
	sourceBefore      *big.Int
	destinationBefore *big.Int
}

var ErrIFTAlreadySubmitted = errors.New("testapp: prepared IFT has already been submitted")

func (i *IFT) Send(ctx context.Context, request IFTRequest) (*IFTTransfer, error) {
	prepared, err := i.Prepare(ctx, request)
	if err != nil {
		return nil, err
	}
	return prepared.Submit(ctx)
}

func (i *IFT) Prepare(ctx context.Context, request IFTRequest) (*PreparedIFT, error) {
	amount, err := validAmount(request.Amount)
	if err != nil {
		return nil, err
	}
	receiver, err := i.receiver(request.Receiver)
	if err != nil {
		return nil, err
	}
	sourceBefore, err := i.balance(ctx, i.source.evm, i.sourceApp, i.sender.Address())
	if err != nil {
		return nil, err
	}
	destinationBefore, err := i.balance(ctx, i.destination.evm, i.destApp, receiver)
	if err != nil {
		return nil, err
	}
	timeoutTimestamp, err := i.timeoutTimestamp(ctx, request.Timeout)
	if err != nil {
		return nil, err
	}
	request.Amount = amount
	request.Receiver = receiver.Hex()
	return &PreparedIFT{
		app:               i,
		request:           request,
		sourceBefore:      sourceBefore,
		destinationBefore: destinationBefore,
		timeoutTimestamp:  timeoutTimestamp,
	}, nil
}

// Submit consumes the prepared transfer before broadcasting. A failed attempt
// cannot be retried because its balance baselines may no longer be current.
func (p *PreparedIFT) Submit(ctx context.Context) (*IFTTransfer, error) {
	if p.submitted {
		return nil, ErrIFTAlreadySubmitted
	}
	p.submitted = true

	data, err := iftABI.Pack(
		"sendTransfer",
		string(p.app.routeID),
		p.request.Receiver,
		p.request.Amount,
		new(big.Int).SetUint64(p.timeoutTimestamp),
	)
	if err != nil {
		return nil, fmt.Errorf("testapp: pack IFT sendTransfer: %w", err)
	}
	txHash, sequence, err := send(
		ctx,
		p.app.source.evm,
		p.app.sender,
		p.app.sourceApp,
		data,
		iftABI,
		eventIFTSent,
	)
	if err != nil {
		return nil, fmt.Errorf("testapp: send IFT on route %q: %w", p.app.routeID, err)
	}
	return &IFTTransfer{
		app: p.app,
		packet: Packet{
			RouteID:      p.app.routeID,
			Source:       p.app.source.chain.ID(),
			SourceTxHash: txHash,
			Sequence:     sequence,
			application:  ApplicationIFT,
		},
		receiver:          common.HexToAddress(p.request.Receiver),
		amount:            new(big.Int).Set(p.request.Amount),
		sourceBefore:      new(big.Int).Set(p.sourceBefore),
		destinationBefore: new(big.Int).Set(p.destinationBefore),
	}, nil
}

func (t *IFTTransfer) Packet() Packet { return t.packet }

// VerifyDelivered waits for the destination effect and checks that exactly the
// submitted amount was minted to the submitted receiver.
func (t *IFTTransfer) VerifyDelivered(ctx context.Context) error {
	received, err := t.awaitReceived(ctx)
	if err != nil {
		return err
	}
	if received.Receiver != t.receiver {
		return fmt.Errorf(
			"testapp: IFT packet %s receiver: got %s, want %s",
			t.packet.reference(),
			received.Receiver.Hex(),
			t.receiver.Hex(),
		)
	}
	if received.Amount.Cmp(t.amount) != 0 {
		return fmt.Errorf(
			"testapp: IFT packet %s amount: got %s, want %s",
			t.packet.reference(),
			received.Amount,
			t.amount,
		)
	}
	want := new(big.Int).Add(t.destinationBefore, t.amount)
	return t.verifyBalance(ctx, t.app.destination.evm, t.app.destApp, t.receiver, want, "delivered")
}

func (t *IFTTransfer) VerifyEscrowed(ctx context.Context) error {
	want := new(big.Int).Sub(t.sourceBefore, t.amount)
	return t.verifyBalance(ctx, t.app.source.evm, t.app.sourceApp, t.app.sender.Address(), want, "escrowed")
}

func (t *IFTTransfer) VerifyNotMinted(ctx context.Context) error {
	return t.verifyBalance(
		ctx,
		t.app.destination.evm,
		t.app.destApp,
		t.receiver,
		t.destinationBefore,
		"not minted",
	)
}

// VerifyRefunded waits for the source refund effect and checks that the source
// holder's original balance has been restored.
func (t *IFTTransfer) VerifyRefunded(ctx context.Context) error {
	refunded, err := t.awaitRefunded(ctx)
	if err != nil {
		return err
	}
	if refunded.Sender != t.app.sender.Address() {
		return fmt.Errorf(
			"testapp: IFT packet %s refund sender: got %s, want %s",
			t.packet.reference(),
			refunded.Sender.Hex(),
			t.app.sender.Address().Hex(),
		)
	}
	if refunded.Amount.Cmp(t.amount) != 0 {
		return fmt.Errorf(
			"testapp: IFT packet %s refund amount: got %s, want %s",
			t.packet.reference(),
			refunded.Amount,
			t.amount,
		)
	}
	return t.verifyBalance(
		ctx,
		t.app.source.evm,
		t.app.sourceApp,
		t.app.sender.Address(),
		t.sourceBefore,
		"refunded",
	)
}

func validAmount(amount *big.Int) (*big.Int, error) {
	if amount == nil {
		return nil, errors.New("testapp: IFT amount is required")
	}
	owned := new(big.Int).Set(amount)
	if owned.Sign() <= 0 {
		return nil, fmt.Errorf("testapp: IFT amount must be positive, got %s", owned)
	}
	if owned.BitLen() > 256 {
		return nil, fmt.Errorf("testapp: IFT amount %s exceeds uint256", owned)
	}
	return owned, nil
}

func (i *IFT) receiver(value string) (common.Address, error) {
	if value != "" {
		return address("IFT receiver", value)
	}
	account, err := evm.NewAccount()
	if err != nil {
		return common.Address{}, fmt.Errorf("testapp: generate IFT receiver: %w", err)
	}
	return account.Address(), nil
}

func (i *IFT) timeoutTimestamp(ctx context.Context, timeout time.Duration) (uint64, error) {
	if timeout <= 0 {
		return 0, nil
	}
	header, err := i.destination.evm.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf(
			"testapp: read destination Chain %q head for IFT timeout: %w",
			i.destination.chain.ID(),
			err,
		)
	}
	seconds := uint64((timeout + time.Second - 1) / time.Second)
	return header.Time + seconds, nil
}

func (i *IFT) balance(
	ctx context.Context,
	client *environment.EVM,
	contract, holder common.Address,
) (*big.Int, error) {
	return callUint(ctx, client, iftABI, contract, "balanceOf", holder)
}

func (t *IFTTransfer) verifyBalance(
	ctx context.Context,
	client *environment.EVM,
	contract, holder common.Address,
	want *big.Int,
	state string,
) error {
	got, err := t.app.balance(ctx, client, contract, holder)
	if err != nil {
		return err
	}
	if got.Cmp(want) != 0 {
		return fmt.Errorf(
			"testapp: IFT packet %s %s balance of %s: got %s, want %s",
			t.packet.reference(),
			state,
			holder.Hex(),
			got,
			want,
		)
	}
	return nil
}

type iftReceivedLog struct {
	RouteID  string `abi:"routeId"`
	Seq      *big.Int
	Receiver common.Address
	Amount   *big.Int
}

func (t *IFTTransfer) awaitReceived(ctx context.Context) (iftReceivedLog, error) {
	definition := iftABI.Events[eventIFTReceived]
	description := fmt.Sprintf("IFT delivery for packet %s", t.packet.reference())
	return awaitEvent(
		ctx,
		t.app.destination.chain,
		t.app.destination.evm,
		t.app.destApp,
		definition.ID,
		description,
		func(data []byte) (iftReceivedLog, error) {
			var event iftReceivedLog
			if err := iftABI.UnpackIntoInterface(&event, eventIFTReceived, data); err != nil {
				return iftReceivedLog{}, fmt.Errorf("testapp: decode IFTReceived: %w", err)
			}
			return event, nil
		},
		func(event iftReceivedLog) bool {
			return event.RouteID == string(t.packet.RouteID) && event.Seq.Uint64() == t.packet.Sequence
		},
	)
}

type iftRefundedLog struct {
	Seq    *big.Int
	Sender common.Address
	Amount *big.Int
}

func (t *IFTTransfer) awaitRefunded(ctx context.Context) (iftRefundedLog, error) {
	definition := iftABI.Events[eventIFTRefunded]
	description := fmt.Sprintf("IFT refund for packet %s", t.packet.reference())
	return awaitEvent(
		ctx,
		t.app.source.chain,
		t.app.source.evm,
		t.app.sourceApp,
		definition.ID,
		description,
		func(data []byte) (iftRefundedLog, error) {
			var event iftRefundedLog
			if err := iftABI.UnpackIntoInterface(&event, eventIFTRefunded, data); err != nil {
				return iftRefundedLog{}, fmt.Errorf("testapp: decode IFTRefunded: %w", err)
			}
			return event, nil
		},
		func(event iftRefundedLog) bool { return event.Seq.Uint64() == t.packet.Sequence },
	)
}
