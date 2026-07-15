package testapp

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/e2e/internal/harness/chain/evm"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"

	bindings "github.com/cosmos/ibc/link/testappbindings"
)

const (
	eventIFTSent     = "IFTSent"
	eventIFTReceived = "IFTReceived"
	eventIFTRefunded = "IFTRefunded"
)

var iftABI = mustABI(bindings.MockIFTMetaData)

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
		iftSentSequence(p.app.sourceApp),
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
	var balance *big.Int
	err := client.UseContractCaller(func(caller bind.ContractCaller) error {
		bound, err := bindings.NewMockIFTCaller(contract, caller)
		if err != nil {
			return fmt.Errorf("testapp: bind MockIFT %s: %w", contract, err)
		}
		balance, err = bound.BalanceOf(&bind.CallOpts{Context: ctx}, holder)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("testapp: query MockIFT balance of %s: %w", holder, err)
	}
	return balance, nil
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

func (t *IFTTransfer) awaitReceived(ctx context.Context) (*bindings.MockIFTIFTReceived, error) {
	definition := iftABI.Events[eventIFTReceived]
	parser := mustBinding(bindings.NewMockIFTFilterer(t.app.destApp, nil))
	return awaitPacketEvent(
		ctx,
		eventSource{endpoint: t.app.destination, contract: t.app.destApp},
		definition.ID,
		"IFT",
		func(log types.Log) (*bindings.MockIFTIFTReceived, error) {
			event, err := parser.ParseIFTReceived(log)
			if err != nil {
				return nil, fmt.Errorf("testapp: decode IFTReceived: %w", err)
			}
			return event, nil
		},
		t.packet,
		func(event *bindings.MockIFTIFTReceived) (string, *big.Int) {
			return event.RouteId, event.Seq
		},
	)
}

func (t *IFTTransfer) awaitRefunded(ctx context.Context) (*bindings.MockIFTIFTRefunded, error) {
	definition := iftABI.Events[eventIFTRefunded]
	parser, err := bindings.NewMockIFTFilterer(t.app.sourceApp, nil)
	if err != nil {
		return nil, fmt.Errorf("testapp: bind source MockIFT events: %w", err)
	}
	description := fmt.Sprintf("IFT refund for packet %s", t.packet.reference())
	return awaitEvent(
		ctx,
		eventSource{endpoint: t.app.source, contract: t.app.sourceApp},
		definition.ID,
		description,
		func(log types.Log) (*bindings.MockIFTIFTRefunded, error) {
			event, err := parser.ParseIFTRefunded(log)
			if err != nil {
				return nil, fmt.Errorf("testapp: decode IFTRefunded: %w", err)
			}
			return event, nil
		},
		func(event *bindings.MockIFTIFTRefunded) bool { return event.Seq.Uint64() == t.packet.Sequence },
	)
}

func iftSentSequence(address common.Address) func(*types.Receipt) (uint64, bool, error) {
	parser := mustBinding(bindings.NewMockIFTFilterer(address, nil))
	return func(receipt *types.Receipt) (uint64, bool, error) {
		definition := iftABI.Events[eventIFTSent]
		for _, log := range receipt.Logs {
			if log.Address != address || len(log.Topics) == 0 || log.Topics[0] != definition.ID {
				continue
			}
			event, err := parser.ParseIFTSent(*log)
			if err != nil {
				return 0, false, fmt.Errorf("testapp: decode IFTSent: %w", err)
			}
			return event.Seq.Uint64(), true, nil
		}
		return 0, false, nil
	}
}
