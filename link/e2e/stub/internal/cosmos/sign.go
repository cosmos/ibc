package cosmos

import (
	"context"
	"fmt"

	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
)

// signMsgs signs and marshals a tx carrying the given messages (SIGN_MODE_DIRECT) with the escrow as the sole
// signer, ready to broadcast. gas is a parameter because module execution (client create + a MsgRecvPacket
// driving the ICS-27 inner tx) needs a far larger budget than a bare bank send; fees stay empty because min
// gas price is 0astake and the feemarket base fee is disabled.
func (c *Client) signMsgs(msgs []sdk.Msg, gas, accountNumber, sequence uint64) ([]byte, error) {
	txBuilder := txConfig.NewTxBuilder()
	if err := txBuilder.SetMsgs(msgs...); err != nil {
		return nil, fmt.Errorf("cosmos: set tx msgs: %w", err)
	}
	txBuilder.SetGasLimit(gas)
	// Fee amount left empty: min gas price is 0astake and the feemarket base fee is disabled.

	pubKey := c.signer.privKey.PubKey()
	// SIGN_MODE_DIRECT derives its sign bytes from the AuthInfo SignerInfos, so a signature carrying the
	// pubkey (with a nil signature) must be set BEFORE the sign bytes are generated; the real signature then
	// overwrites it.
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
		txBuilder, c.signer.privKey, txConfig, sequence,
	)
	if err != nil {
		return nil, fmt.Errorf("cosmos: sign tx: %w", err)
	}
	if signErr := txBuilder.SetSignatures(sigV2); signErr != nil {
		return nil, fmt.Errorf("cosmos: set signature: %w", signErr)
	}

	txBytes, err := txConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return nil, fmt.Errorf("cosmos: encode tx: %w", err)
	}
	return txBytes, nil
}
