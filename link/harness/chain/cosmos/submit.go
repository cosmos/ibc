package cosmos

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	"github.com/cosmos/ibc/link/harness/chain"
	"github.com/cosmos/ibc/link/harness/fixturekeys"
	"github.com/cosmos/ibc/link/harness/ibclink/wire"
	"github.com/cosmos/ibc/link/harness/onchain"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkbech32 "github.com/cosmos/cosmos-sdk/types/bech32"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	gmptypes "github.com/cosmos/ibc-go/v11/modules/apps/27-gmp/types"
	ifttypes "github.com/cosmos/ibc-go/v11/modules/apps/prototypes/ift/types"
	channelv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
)

// This file implements Cosmos app submission with SIGN_MODE_DIRECT using the user/faucet key.

const (
	// gmpGasLimit covers 27-gmp module execution and packet commitment, including the native IFT wrapper.
	gmpGasLimit = 1_000_000
	// packetTimeoutWindow is the default future deadline for native IFT and GMP packets. An hour outlives
	// happy-path tests and stays under channel v2's max timeout delta.
	packetTimeoutWindow = time.Hour
)

// signTxConfig is the SIGN_MODE_DIRECT-only signing config over the package's shared protoCodec.
var signTxConfig = authtx.NewTxConfig(protoCodec, []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_DIRECT})

// signer holds the parsed user/faucet key as an sdk secp256k1 private key plus its derived bech32 account
// address (the MsgIFTTransfer / MsgSendCall signer). The stub derives the same address from the same key
// independently; the two agree by construction.
type signer struct {
	privKey *secp256k1.PrivKey
	address string
}

// newSigner parses a plain-secp256k1 hex key (with or without 0x) into an sdk secp256k1.PrivKey and derives
// its cosmos bech32 address — the standard RIPEMD160(SHA256(compressed-pubkey)) derivation, bech32-encoded
// under the "cosmos" HRP with the explicit encoder (never the global sealable prefix config).
func newSigner(hexKey string) (signer, error) {
	raw, err := hex.DecodeString(strings.TrimPrefix(hexKey, "0x"))
	if err != nil {
		return signer{}, fmt.Errorf("cosmos: parse signer key: %w", err)
	}
	if len(raw) != secp256k1.PrivKeySize {
		return signer{}, fmt.Errorf("cosmos: signer key must be %d bytes, got %d", secp256k1.PrivKeySize, len(raw))
	}
	privKey := &secp256k1.PrivKey{Key: raw}
	addr, err := sdkbech32.ConvertAndEncode(Bech32HRP, privKey.PubKey().Address())
	if err != nil {
		return signer{}, fmt.Errorf("cosmos: encode signer address: %w", err)
	}
	return signer{privKey: privKey, address: addr}, nil
}

// Submitter is one cosmos source chain's AppSubmitter, bound at construction to that chain's client, its
// deployed fixtures (the IFT denom and source attestations client), and its timing budget —
// the same binding shape as the read-side reader.
type Submitter struct {
	client       *Client
	iftDenom     string // the native tokenfactory denom registered with the IFT module
	sourceClient string // the 27-gmp source attestations client id (the AttestationsClient fixture)
	budget       onchain.Budget
}

var _ chain.AppSubmitter = (*Submitter)(nil)

// NewSubmitter builds a cosmos AppSubmitter over one chain's client and deployment.
func NewSubmitter(c *Client, dep wire.ChainDeployment, budget onchain.Budget) (*Submitter, error) {
	iftDenom, err := dep.Fixture(fixturekeys.IFTDenom)
	if err != nil {
		return nil, fmt.Errorf("cosmos submit: %w", err)
	}
	sourceClient, err := dep.Fixture(fixturekeys.AttestationsClient)
	if err != nil {
		return nil, fmt.Errorf("cosmos submit: %w", err)
	}
	return &Submitter{client: c, iftDenom: iftDenom, sourceClient: sourceClient, budget: budget}, nil
}

// SubmitIFT burns the source token through the native IFT module and returns the real IBC packet sequence.
func (s *Submitter) SubmitIFT(ctx context.Context, in chain.IFTSubmission) (chain.AppSubmitResult, error) {
	timeoutTimestamp := in.TimeoutTimestamp
	if timeoutTimestamp == 0 {
		timeoutTimestamp = uint64(time.Now().Add(packetTimeoutWindow).Unix())
	}
	msg := &ifttypes.MsgIFTTransfer{
		Signer:           s.client.signer.address,
		Denom:            s.iftDenom,
		ClientId:         s.sourceClient,
		Receiver:         in.Receiver,
		Amount:           sdkmath.NewIntFromBigInt(in.Amount),
		TimeoutTimestamp: timeoutTimestamp,
	}
	res, err := s.submit(ctx, msg, gmpGasLimit)
	if err != nil {
		return chain.AppSubmitResult{}, err
	}
	seq, ok := iftSeqFromEvents(res.TxResult.Events, s.sourceClient)
	if !ok {
		return chain.AppSubmitResult{}, fmt.Errorf(
			"cosmos submit: MsgIFTTransfer tx %s emitted no matching %s/%s sequence for source client %s",
			res.Hash, ifttypes.EventTypeIFTTransferInitiated, channelv2.EventTypeSendPacket, s.sourceClient,
		)
	}
	return chain.AppSubmitResult{SourceTxHash: res.Hash.String(), Sequence: seq}, nil
}

// SubmitGMP sends a 27-gmp MsgSendCall and returns its module-assigned packet sequence.
func (s *Submitter) SubmitGMP(ctx context.Context, in chain.GMPSubmission) (chain.AppSubmitResult, error) {
	msg := &gmptypes.MsgSendCall{
		SourceClient:     s.sourceClient,
		Sender:           s.client.signer.address,
		Receiver:         in.Target,
		Salt:             nil,
		Payload:          in.Payload,
		TimeoutTimestamp: uint64(time.Now().Add(packetTimeoutWindow).Unix()),
		Encoding:         gmptypes.EncodingABI,
	}
	res, err := s.submit(ctx, msg, gmpGasLimit)
	if err != nil {
		return chain.AppSubmitResult{}, err
	}
	seq, ok := sendSeqFromEvents(res.TxResult.Events, s.sourceClient)
	if !ok {
		return chain.AppSubmitResult{}, fmt.Errorf(
			"cosmos submit: MsgSendCall tx %s emitted no %s.%s for source client %s",
			res.Hash, channelv2.EventTypeSendPacket, channelv2.AttributeKeySequence, s.sourceClient,
		)
	}
	return chain.AppSubmitResult{SourceTxHash: res.Hash.String(), Sequence: seq}, nil
}

// submit signs msg as the faucet, broadcasts it, and waits for it to commit successfully, returning the
// committed tx (hash + result events).
func (s *Submitter) submit(ctx context.Context, msg sdk.Msg, gas uint64) (*coretypes.ResultTx, error) {
	accountNumber, sequence, err := s.client.account(ctx)
	if err != nil {
		return nil, err
	}
	txBytes, err := s.client.signMsg(msg, gas, accountNumber, sequence)
	if err != nil {
		return nil, err
	}
	hash, err := s.client.broadcast(ctx, txBytes)
	if err != nil {
		return nil, err
	}
	return s.waitTx(ctx, hash)
}

// waitTx polls the tx index (bounded by the chain's budget) until hash is committed with a successful
// result, returning the committed tx. A committed tx with a non-zero result code is a submission that
// reverted on-chain — a hard error.
func (s *Submitter) waitTx(ctx context.Context, hexHash string) (*coretypes.ResultTx, error) {
	raw, err := hex.DecodeString(hexHash)
	if err != nil {
		return nil, fmt.Errorf("cosmos: decode tx hash %q: %w", hexHash, err)
	}
	desc := fmt.Sprintf("submitted tx %s inclusion", hexHash)
	return onchain.Await(ctx, s.budget.Completion, s.budget.Poll, desc,
		func(ctx context.Context) (*coretypes.ResultTx, bool, error) {
			res, err := s.client.comet.Tx(ctx, raw, false)
			if err != nil {
				return nil, false, err // typically "not found" until committed; retry within the budget
			}
			if res.TxResult.Code != 0 {
				return nil, true, fmt.Errorf(
					"cosmos: submitted tx %s reverted (code %d): %s", hexHash, res.TxResult.Code, res.TxResult.Log,
				)
			}
			return res, true, nil
		})
}

// signMsg signs and marshals a tx carrying msg (SIGN_MODE_DIRECT) with the faucet as the sole signer. gas
// is a parameter because module calls have different execution costs; fees stay empty (min gas price
// 0astake, feemarket base fee disabled).
func (c *Client) signMsg(msg sdk.Msg, gas, accountNumber, sequence uint64) ([]byte, error) {
	txBuilder := signTxConfig.NewTxBuilder()
	if err := txBuilder.SetMsgs(msg); err != nil {
		return nil, fmt.Errorf("cosmos: set tx msg: %w", err)
	}
	txBuilder.SetGasLimit(gas)

	pubKey := c.signer.privKey.PubKey()
	// SIGN_MODE_DIRECT derives its sign bytes from the AuthInfo SignerInfos, so prime a signature carrying
	// the pubkey (with a nil signature) before generating the sign bytes; the real signature overwrites it.
	if err := txBuilder.SetSignatures(signingtypes.SignatureV2{
		PubKey:   pubKey,
		Data:     &signingtypes.SingleSignatureData{SignMode: signingtypes.SignMode_SIGN_MODE_DIRECT},
		Sequence: sequence,
	}); err != nil {
		return nil, fmt.Errorf("cosmos: prime signer info: %w", err)
	}

	signerData := authsigning.SignerData{
		ChainID:       c.chainID,
		AccountNumber: accountNumber,
		Sequence:      sequence,
		PubKey:        pubKey,
		Address:       c.signer.address,
	}
	sigV2, err := clienttx.SignWithPrivKey(
		context.Background(), signingtypes.SignMode_SIGN_MODE_DIRECT, signerData,
		txBuilder, c.signer.privKey, signTxConfig, sequence,
	)
	if err != nil {
		return nil, fmt.Errorf("cosmos: sign tx: %w", err)
	}
	if signErr := txBuilder.SetSignatures(sigV2); signErr != nil {
		return nil, fmt.Errorf("cosmos: set signature: %w", signErr)
	}

	txBytes, err := signTxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return nil, fmt.Errorf("cosmos: encode tx: %w", err)
	}
	return txBytes, nil
}

// account reads the faucet account's number and sequence over the typed gRPC auth QueryClient.
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

// broadcast submits a signed tx via broadcast_tx_sync and returns its hash (uppercase hex). A non-zero
// CheckTx code means the tx was rejected before entering the mempool (bad signature, insufficient funds).
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

// sendSeqFromEvents reads the module-assigned packet sequence from a MsgSendCall tx's channelv2 send_packet
// event, matched to the source client so the sequence is read unambiguously.
func sendSeqFromEvents(events []abci.Event, sourceClient string) (uint64, bool) {
	for _, ev := range events {
		if ev.Type != channelv2.EventTypeSendPacket {
			continue
		}
		attrs := attrMap(ev)
		if attrs[channelv2.AttributeKeySrcClient] != sourceClient {
			continue
		}
		seq, err := strconv.ParseUint(attrs[channelv2.AttributeKeySequence], 10, 64)
		if err != nil {
			return 0, false
		}
		return seq, true
	}
	return 0, false
}

// iftSeqFromEvents reads the core packet identity and cross-checks the native IFT application event.
func iftSeqFromEvents(events []abci.Event, sourceClient string) (uint64, bool) {
	sent, ok := sendSeqFromEvents(events, sourceClient)
	if !ok {
		return 0, false
	}
	for _, ev := range events {
		if ev.Type != ifttypes.EventTypeIFTTransferInitiated {
			continue
		}
		attrs := attrMap(ev)
		if attrs[ifttypes.AttributeKeyClientID] != sourceClient {
			continue
		}
		seq, err := strconv.ParseUint(attrs[ifttypes.AttributeKeySequence], 10, 64)
		if err != nil {
			return 0, false
		}
		return sent, seq == sent
	}
	return 0, false
}
