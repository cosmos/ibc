package cosmos

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/address"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	abci "github.com/cometbft/cometbft/abci/types"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	txsigning "github.com/cosmos/cosmos-sdk/x/tx/signing"
	gogoproto "github.com/cosmos/gogoproto/proto"
	gmptypes "github.com/cosmos/ibc-go/v11/modules/apps/27-gmp/types"
	ifttypes "github.com/cosmos/ibc-go/v11/modules/apps/prototypes/ift/types"
	tokenfactorytypes "github.com/cosmos/ibc-go/v11/modules/apps/prototypes/tokenfactory/types"
	channelv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

// protoCodec backs the typed gRPC queries and SIGN_MODE_DIRECT submission config. Its interface registry
// covers every native application message the harness signs.
var protoCodec = newProtoCodec()

func newProtoCodec() *codec.ProtoCodec {
	registry, err := codectypes.NewInterfaceRegistryWithOptions(codectypes.InterfaceRegistryOptions{
		ProtoFiles: gogoproto.HybridResolver,
		SigningOptions: txsigning.Options{
			AddressCodec:          address.NewBech32Codec(Bech32HRP),
			ValidatorAddressCodec: address.NewBech32Codec(Bech32HRP + "valoper"),
		},
	})
	if err != nil {
		panic(fmt.Sprintf("cosmos: build interface registry: %v", err))
	}
	cryptocodec.RegisterInterfaces(registry)
	banktypes.RegisterInterfaces(registry)
	channelv2.RegisterInterfaces(registry)
	gmptypes.RegisterInterfaces(registry)
	ifttypes.RegisterInterfaces(registry)
	tokenfactorytypes.RegisterInterfaces(registry)
	return codec.NewProtoCodec(registry)
}

// Client is the harness's client for a cosmos chain: it observes outcomes over CometBFT tx_search and
// typed gRPC bank queries, and submits the source-side user actions signed by the chain's user/faucet key
// (see submit.go and chain.AppSubmitter). The relayer owns every delivery leg on its own side of the wall.
type Client struct {
	comet   *rpchttp.HTTP
	grpc    *grpc.ClientConn
	chainID string // CometBFT chain-id, needed to sign source submissions (SIGN_MODE_DIRECT sign bytes)
	signer  signer // the user/faucet signer for source submissions (the relayer/admin signer is the SUT's)
}

// NewClient builds a client from a cosmos chain's CometBFT RPC URL and gRPC dial target, plus the chain-id
// and the user/faucet key it signs source submissions with. The CometBFT client is constructed (never
// dialed until Start, which is never called — tx_search rides the HTTP caller); the gRPC conn is lazy
// (grpc.NewClient connects on first query) and carries the SDK proto codec forced as its call codec so
// gogoproto query messages marshal correctly.
func NewClient(cometURL, grpcURL, chainID, faucetKeyHex string) (*Client, error) {
	sg, err := newSigner(faucetKeyHex)
	if err != nil {
		return nil, err
	}
	comet, err := rpchttp.New(cometURL, "/websocket")
	if err != nil {
		return nil, fmt.Errorf("cosmos: build comet rpc client: %w", err)
	}
	conn, err := grpc.NewClient(
		grpcURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(protoCodec.GRPCCodec())),
	)
	if err != nil {
		return nil, fmt.Errorf("cosmos: build grpc client: %w", err)
	}
	return &Client{comet: comet, grpc: conn, chainID: chainID, signer: sg}, nil
}

// Close releases the gRPC conn.
func (c *Client) Close() error { return c.grpc.Close() }

// Height reports the chain's latest committed block height (CometBFT /status).
func (c *Client) Height(ctx context.Context) (uint64, error) {
	status, err := c.comet.Status(ctx)
	if err != nil {
		return 0, fmt.Errorf("cosmos: comet status: %w", err)
	}
	return uint64(status.SyncInfo.LatestBlockHeight), nil
}

// IFTRecv is one native IFT mint correlated with its IBC acknowledgement in the same transaction.
type IFTRecv struct {
	Seq      uint64
	Receiver string
	Amount   *big.Int
	Denom    string
}

// IFTRecvs returns successful native IFT mints for destClient. Sequence identity comes from the real
// write_acknowledgement event emitted in the same transaction as ift_mint_received.
func (c *Client) IFTRecvs(ctx context.Context, destClient string) ([]IFTRecv, error) {
	query := ifttypes.EventTypeIFTMintReceived + "." + ifttypes.AttributeKeyClientID + "='" + destClient + "'"
	txs, err := c.txSearchAll(ctx, query)
	if err != nil {
		return nil, err
	}

	var out []IFTRecv
	for _, tx := range txs {
		if tx.TxResult.Code != 0 {
			continue
		}
		receiver, amount, denom, ok, err := iftMintFromEvents(tx.TxResult.Events, destClient)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		for _, ev := range tx.TxResult.Events {
			if ev.Type != channelv2.EventTypeWriteAck {
				continue
			}
			attrs := attrMap(ev)
			if attrs[channelv2.AttributeKeyDstClient] != destClient {
				continue
			}
			seq, parseErr := strconv.ParseUint(attrs[channelv2.AttributeKeySequence], 10, 64)
			if parseErr != nil {
				return nil, fmt.Errorf(
					"cosmos: write_acknowledgement sequence %q: %w",
					attrs[channelv2.AttributeKeySequence],
					parseErr,
				)
			}
			success, ackErr := ackSuccess(attrs[channelv2.AttributeKeyEncodedAckHex])
			if ackErr != nil {
				return nil, ackErr
			}
			if success {
				out = append(out, IFTRecv{Seq: seq, Receiver: receiver, Amount: amount, Denom: denom})
			}
		}
	}
	return out, nil
}

// IFTRefund is one native IFT error-acknowledgement or timeout refund.
type IFTRefund struct {
	Seq    uint64
	Amount *big.Int
	Denom  string
}

// IFTRefunds returns the native IFT module's source-side refund events for sourceClient.
func (c *Client) IFTRefunds(ctx context.Context, sourceClient string) ([]IFTRefund, error) {
	query := ifttypes.EventTypeIFTTransferRefunded + "." + ifttypes.AttributeKeyClientID + "='" + sourceClient + "'"
	txs, err := c.txSearchAll(ctx, query)
	if err != nil {
		return nil, err
	}
	var out []IFTRefund
	for _, tx := range txs {
		if tx.TxResult.Code != 0 {
			continue
		}
		for _, ev := range tx.TxResult.Events {
			if ev.Type != ifttypes.EventTypeIFTTransferRefunded {
				continue
			}
			attrs := attrMap(ev)
			if attrs[ifttypes.AttributeKeyClientID] != sourceClient {
				continue
			}
			seq, parseErr := strconv.ParseUint(attrs[ifttypes.AttributeKeySequence], 10, 64)
			if parseErr != nil {
				return nil, fmt.Errorf(
					"cosmos: IFT refund sequence %q: %w",
					attrs[ifttypes.AttributeKeySequence],
					parseErr,
				)
			}
			amount, ok := new(big.Int).SetString(attrs[ifttypes.AttributeKeyAmount], 10)
			if !ok {
				return nil, fmt.Errorf(
					"cosmos: IFT refund amount %q is not a base-10 integer",
					attrs[ifttypes.AttributeKeyAmount],
				)
			}
			out = append(out, IFTRefund{Seq: seq, Amount: amount, Denom: attrs[ifttypes.AttributeKeyDenom]})
		}
	}
	return out, nil
}

// GMPRecv is one GMP packet the harness matched via the chain's real IBC v2 write_acknowledgement event: the
// packet sequence, whether the acknowledgement was a success (vs the universal error ack), and the increment
// target read from the module's inner bank transfer. On success Target is the recipient of the ICS-27 executor
// account's counter-denom send (the real +1); on an error ack nothing moved, so Target is empty and the caller
// substitutes the fixture counter target.
type GMPRecv struct {
	Seq     uint64
	Success bool
	Target  string
}

// GMPRecvs returns every committed GMP recv on destClient, read from the chain's 27-gmp/IBC v2 events. It
// tx_searches the write_acknowledgement events for the destination client,
// decodes each acknowledgement's success bit from the canonical ack bytes, and — on success — reads the
// increment target from the inner bank `transfer` event whose sender is the ICS-27 executor account (ics27)
// and whose denom is the GMP counter denom (gmpDenom). The onchain cosmosReader filters these by sequence.
func (c *Client) GMPRecvs(ctx context.Context, destClient, ics27, gmpDenom string) ([]GMPRecv, error) {
	query := channelv2.EventTypeWriteAck + "." + channelv2.AttributeKeyDstClient + "='" + destClient + "'"
	txs, err := c.txSearchAll(ctx, query)
	if err != nil {
		return nil, err
	}

	var out []GMPRecv
	for _, tx := range txs {
		if tx.TxResult.Code != 0 {
			continue // a failed recv tx wrote no ack; never a delivery
		}
		for _, ev := range tx.TxResult.Events {
			if ev.Type != channelv2.EventTypeWriteAck {
				continue
			}
			attrs := attrMap(ev)
			seq, err := strconv.ParseUint(attrs[channelv2.AttributeKeySequence], 10, 64)
			if err != nil {
				return nil, fmt.Errorf(
					"cosmos: write_acknowledgement sequence %q: %w",
					attrs[channelv2.AttributeKeySequence],
					err,
				)
			}
			success, err := ackSuccess(attrs[channelv2.AttributeKeyEncodedAckHex])
			if err != nil {
				return nil, err
			}
			target := ""
			if success {
				target = transferRecipient(tx.TxResult.Events, ics27, gmpDenom)
			}
			out = append(out, GMPRecv{Seq: seq, Success: success, Target: target})
		}
	}
	return out, nil
}

// txSearchAll returns every tx matching query, paging through the full result set. CometBFT defaults nil
// paging to per_page=30 (oldest first), so a single unpaged call silently drops matches past the 30th once
// a query key accumulates that many committed txs — a correlation guard would then miss a real delivery/ack.
// Paging by TotalCount reads them all, so the idempotency and Await lookups can never miss a committed tx.
func (c *Client) txSearchAll(ctx context.Context, query string) ([]*coretypes.ResultTx, error) {
	const perPage = 100
	var out []*coretypes.ResultTx
	for page := 1; ; page++ {
		p, pp := page, perPage
		res, err := c.comet.TxSearch(ctx, query, false, &p, &pp, "asc")
		if err != nil {
			return nil, fmt.Errorf("cosmos: tx_search: %w", err)
		}
		out = append(out, res.Txs...)
		if len(res.Txs) == 0 || len(out) >= res.TotalCount {
			return out, nil
		}
	}
}

// ackSuccess decodes a hex-encoded channel/v2 Acknowledgement and reports whether it was a success (its first
// app-ack is not the universal error-acknowledgement sentinel) — the canonical, module-emitted success signal.
func ackSuccess(encodedHex string) (bool, error) {
	raw, err := hex.DecodeString(encodedHex)
	if err != nil {
		return false, fmt.Errorf("cosmos: decode acknowledgement hex: %w", err)
	}
	var ack channelv2.Acknowledgement
	if err := ack.Unmarshal(raw); err != nil {
		return false, fmt.Errorf("cosmos: unmarshal acknowledgement: %w", err)
	}
	return ack.Success(), nil
}

// transferRecipient finds the bank `transfer` event whose sender is `from` and whose amount is in `denom`, and
// returns its recipient — the ICS-27 increment's target. Returns "" if no such transfer is present.
func transferRecipient(events []abci.Event, from, denom string) string {
	for _, ev := range events {
		if ev.Type != banktypes.EventTypeTransfer {
			continue
		}
		attrs := attrMap(ev)
		if attrs[banktypes.AttributeKeySender] != from {
			continue
		}
		if _, dn, err := parseCoin(attrs[sdk.AttributeKeyAmount]); err != nil || dn != denom {
			continue
		}
		return attrs[banktypes.AttributeKeyRecipient]
	}
	return ""
}

// attrMap flattens an ABCI event's attributes into a lookup map.
func attrMap(ev abci.Event) map[string]string {
	m := make(map[string]string, len(ev.Attributes))
	for _, a := range ev.Attributes {
		m[a.Key] = a.Value
	}
	return m
}

// BalanceOf reads holder's bank balance of denom over the typed gRPC bank QueryClient, defaulting to zero
// when the denom is absent (a fresh receiver holds nothing). The amount is an sdkmath.Int, so a >2^53
// balance survives.
func (c *Client) BalanceOf(ctx context.Context, holder, denom string) (*big.Int, error) {
	resp, err := banktypes.NewQueryClient(c.grpc).
		Balance(ctx, &banktypes.QueryBalanceRequest{Address: holder, Denom: denom})
	if err != nil {
		return nil, fmt.Errorf("cosmos: query balance of %s: %w", holder, err)
	}
	if resp.Balance == nil {
		return big.NewInt(0), nil
	}
	return resp.Balance.Amount.BigInt(), nil
}

// iftMintFromEvents reads the native IFT mint effect emitted for destClient.
func iftMintFromEvents(
	events []abci.Event,
	destClient string,
) (receiver string, amount *big.Int, denom string, ok bool, err error) {
	for _, ev := range events {
		if ev.Type != ifttypes.EventTypeIFTMintReceived {
			continue
		}
		attrs := attrMap(ev)
		if attrs[ifttypes.AttributeKeyClientID] != destClient {
			continue
		}
		amt, parsed := new(big.Int).SetString(attrs[ifttypes.AttributeKeyAmount], 10)
		if !parsed {
			return "", nil, "", false, fmt.Errorf(
				"cosmos: IFT mint amount %q is not a base-10 integer",
				attrs[ifttypes.AttributeKeyAmount],
			)
		}
		return attrs[ifttypes.AttributeKeyReceiver], amt, attrs[ifttypes.AttributeKeyDenom], true, nil
	}
	return "", nil, "", false, nil
}

// parseCoin splits a "NNNNdenom" coin string (e.g. "42ugmpc") into its integer amount and denom.
func parseCoin(s string) (*big.Int, string, error) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i == len(s) {
		return nil, "", fmt.Errorf("cosmos: malformed coin %q (want <amount><denom>)", s)
	}
	amt, ok := new(big.Int).SetString(s[:i], 10)
	if !ok {
		return nil, "", fmt.Errorf("cosmos: coin amount %q is not a base-10 integer", s[:i])
	}
	return amt, s[i:], nil
}
