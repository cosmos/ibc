// Package cosmos is the stub's client for the cosmos chain family. It lives under link/internal and never
// imports the harness's cosmos package; the two agree only on the chain's public surfaces and shared literals.
//
// The stub uses it for the write side of a cosmos destination: connect for readiness on `relayer run`,
// create/register native IFT fixtures on `deploy`, and deliver GMP/IFT packets through IBC v2. It signs
// SIGN_MODE_DIRECT with the upstream cosmos-sdk x/auth tx builder over an sdk secp256k1 key.
//
// CometBFT calls go over the official rpc/client/http client; bank and auth reads go over typed gRPC query
// clients (banktypes/authtypes).
package cosmos

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	abci "github.com/cometbft/cometbft/abci/types"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

const (
	// AddressPrefix is the HRP plus the bech32 "1" separator, so every account address begins with it.
	AddressPrefix = "cosmos1"

	// Bech32HRP is the account-address human-readable prefix.
	Bech32HRP = "cosmos"

	// GMPDenom is the bank denom the cosmos GMP "counter" stands in as — the cosmos analog of the solidity
	// Counter fixture, same trust model. There is no Counter contract on a cosmos chain; instead the relayer is
	// genesis-funded with this denom and one increment is exactly one <GMPDenom> relayer->target send, so the
	// target's balance of it is the count. A shared contract with the harness's own copy (mismatch would make
	// increments fail insufficient-funds), so both sides pin the same literal.
	GMPDenom = "ugmpc"

	// GMPIncrementPayload is the fixed GMP call payload the cosmos executor recognizes as "increment the
	// counter" — the cosmos analog of the EVM Counter.increment() calldata. A shared contract with the
	// harness's own copy: the harness reader hands this exact payload as the default GMP payload, and
	// the executor increments only when the delivered payload equals it (any other payload is an error-ack).
	GMPIncrementPayload = "increment"

	// waitTxTimeout / waitTxPoll bound the post-broadcast wait for a delivery tx to be committed. The chain
	// commits blocks at ~650ms, so a handful of blocks is ample; the wait is a bounded condition poll on
	// tx inclusion, never a sleep.
	waitTxTimeout = 30 * time.Second
	waitTxPoll    = 200 * time.Millisecond

	// connectTimeout bounds the readiness probe in Connect.
	connectTimeout = 10 * time.Second
)

// Client talks to one cosmos chain: CometBFT RPC (broadcast/query/search) plus typed gRPC (bank/auth),
// signing with the configured relayer/admin key.
type Client struct {
	comet   *rpchttp.HTTP
	grpc    *grpc.ClientConn
	chainID string
	signer  signer
}

// Connect builds a client for a cosmos chain and probes CometBFT /status as a liveness check (the cosmos
// analog of dialing an EVM RPC and reading its chain id). It derives the relayer signer address from the
// signer key up front so a bad key fails here, not mid-delivery. The gRPC conn is lazy (grpc.NewClient
// connects on first query) and carries the stub's SDK proto codec forced as its call codec so gogoproto
// query messages marshal correctly.
func Connect(ctx context.Context, cometURL, grpcURL, chainID, signerKeyHex string) (*Client, error) {
	sg, err := newSigner(signerKeyHex)
	if err != nil {
		return nil, err
	}
	comet, err := rpchttp.New(strings.TrimRight(cometURL, "/"), "/websocket")
	if err != nil {
		return nil, fmt.Errorf("cosmos: build comet rpc client: %w", err)
	}
	conn, err := grpc.NewClient(
		grpcURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(cdc.GRPCCodec())),
	)
	if err != nil {
		return nil, fmt.Errorf("cosmos: build grpc client: %w", err)
	}
	c := &Client{comet: comet, grpc: conn, chainID: chainID, signer: sg}

	pctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if _, err := c.comet.Status(pctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("cosmos: status probe %s: %w", chainID, err)
	}
	// grpc.NewClient dials lazily, so a wrong grpcUrl would otherwise first surface mid-deploy/-delivery.
	// A cheap bank Params query forces the dial here so a bad gRPC endpoint fails at connect, like /status.
	if _, err := banktypes.NewQueryClient(conn).Params(pctx, &banktypes.QueryParamsRequest{}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("cosmos: grpc probe %s: %w", chainID, err)
	}
	return c, nil
}

// Close releases the gRPC conn.
func (c *Client) Close() error { return c.grpc.Close() }

// SignerAddress is the relayer/admin bech32 account used for deployment and destination transactions.
func (c *Client) SignerAddress() string { return c.signer.address }

// Balance reads holder's bank balance of denom over the typed gRPC bank QueryClient, returning zero when the
// denom is absent.
func (c *Client) Balance(ctx context.Context, holder, denom string) (*big.Int, error) {
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

// account reads the signer account's number and sequence over the typed gRPC auth QueryClient. AccountInfo
// returns a BaseAccount with plain uint64 account_number/sequence — no Any-unpacking and no string parse.
func (c *Client) account(ctx context.Context) (accountNumber, sequence uint64, err error) {
	resp, err := authtypes.NewQueryClient(c.grpc).
		AccountInfo(ctx, &authtypes.QueryAccountInfoRequest{Address: c.signer.address})
	if err != nil {
		return 0, 0, fmt.Errorf("cosmos: query account info %s: %w", c.signer.address, err)
	}
	if resp.Info == nil {
		return 0, 0, fmt.Errorf("cosmos: account %s not found on-chain", c.signer.address)
	}
	return resp.Info.AccountNumber, resp.Info.Sequence, nil
}

// txSearchPerPage is the page size for paginated tx_search. CometBFT defaults nil paging to per_page=30
// and caps it at 100; without paging a lookup would only ever see the oldest 30 matching txs, so an
// idempotency guard could miss a committed delivery/ack once >30 accumulate for a query key.
const txSearchPerPage = 100

// forEachTx pages through every committed tx matching query (oldest first) and invokes visit on each until
// it returns stop=true or the pages are exhausted. It paginates explicitly rather than passing nil paging,
// so a match past the first 30 results is still found.
func (c *Client) forEachTx(
	ctx context.Context,
	query string,
	visit func(*coretypes.ResultTx) (stop bool, err error),
) error {
	perPage := txSearchPerPage
	for page := 1; ; page++ {
		p := page
		res, err := c.comet.TxSearch(ctx, query, false, &p, &perPage, "asc")
		if err != nil {
			return fmt.Errorf("cosmos: tx_search: %w", err)
		}
		for _, tx := range res.Txs {
			stop, err := visit(tx)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
		}
		if len(res.Txs) == 0 || page*perPage >= res.TotalCount {
			return nil
		}
	}
}

// broadcast submits a signed tx via broadcast_tx_sync and returns its hash (uppercase hex). A non-zero
// CheckTx code means the tx was rejected before entering the mempool (bad signature, insufficient funds),
// which is a delivery failure, not a transient hiccup.
func (c *Client) broadcast(ctx context.Context, txBytes []byte) (string, error) {
	resp, err := c.comet.BroadcastTxSync(ctx, txBytes)
	if err != nil {
		return "", fmt.Errorf("cosmos: broadcast_tx_sync: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("cosmos: broadcast rejected (CheckTx code %d): %s", resp.Code, resp.Log)
	}
	return resp.Hash.String(), nil
}

// waitTx polls the tx index until the hash is committed with a successful result, or the bounded budget
// elapses. A not-yet-committed tx makes the tx query error; that is retried within the budget. A committed
// tx with a non-zero result code is a delivery that reverted on-chain — a hard error, never retried away.
func (c *Client) waitTx(ctx context.Context, hexHash string) error {
	raw, err := hex.DecodeString(hexHash)
	if err != nil {
		return fmt.Errorf("cosmos: decode tx hash %q: %w", hexHash, err)
	}

	ctx, cancel := context.WithTimeout(ctx, waitTxTimeout)
	defer cancel()
	ticker := time.NewTicker(waitTxPoll)
	defer ticker.Stop()
	var lastErr error
	for {
		if res, err := c.comet.Tx(ctx, raw, false); err != nil {
			lastErr = err // typically "not found" until committed; retry within the budget
		} else if res.TxResult.Code != 0 {
			return fmt.Errorf(
				"cosmos: delivery tx %s reverted (code %d): %s",
				hexHash,
				res.TxResult.Code,
				res.TxResult.Log,
			)
		} else {
			return nil
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("cosmos: awaiting tx %s inclusion: %w (last: %w)", hexHash, ctx.Err(), lastErr)
			}
			return fmt.Errorf("cosmos: awaiting tx %s inclusion: %w", hexHash, ctx.Err())
		case <-ticker.C:
		}
	}
}

// txEvents fetches a committed tx's result events by hash (CometBFT `tx` RPC), typed as abci.Event — the same
// event type ibcv2.go's classifiers read.
func (c *Client) txEvents(ctx context.Context, hexHash string) ([]abci.Event, error) {
	raw, err := hex.DecodeString(hexHash)
	if err != nil {
		return nil, fmt.Errorf("cosmos: decode tx hash %q: %w", hexHash, err)
	}
	res, err := c.comet.Tx(ctx, raw, false)
	if err != nil {
		return nil, fmt.Errorf("cosmos: fetch tx events: %w", err)
	}
	return res.TxResult.Events, nil
}
