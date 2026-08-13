// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"strings"
	"time"

	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ift"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/cosmos/ibc/link/internal/config"
)

var (
	cmdTxIFT = &cobra.Command{
		Use:   useIFT,
		Short: "IFT transaction subcommands",
	}

	cmdTxIFTMint = &cobra.Command{
		Use:   "mint",
		Short: "Mint IFT supply to an address",
		Long:  "Mint --amount of the IFT token at --ift to --to. The --from signer must be the token's owner.",
		Example: "  ibc tx ift mint --chain 1 --ift 0xIFTTokenAddress... \\\n" +
			"    --to 0xRecipient... --amount 1000000000000000000 --from deployer",
		RunE: txIFTMint,
	}

	cmdTxIFTSend = &cobra.Command{
		Use:   "send",
		Short: "Send IFT across a bridge to a counterparty chain",
		Long: "Initiate a cross-chain transfer of --amount of the IFT token at --ift, over the bridge " +
			"registered for --client-id, to --to on the counterparty chain.",
		Example: "  ibc tx ift send --chain 1 --ift 0xIFTTokenAddress... --client-id link-1-2 \\\n" +
			"    --to 0xReceiver... --amount 500000000000000000 --from deployer",
		RunE: txIFTSend,
	}
)

var (
	flagTxIFTChain    string
	flagTxIFTAddress  string
	flagTxIFTFrom     string
	flagTxIFTTimeout  time.Duration
	flagTxIFTTo       string
	flagTxIFTAmount   string
	flagTxIFTClientID string
)

func txIFTMint(cmd *cobra.Command, _ []string) error {
	amount := parseIFTAmount(flagTxIFTAmount)
	if amount == nil {
		return errors.Errorf("invalid --amount %q", flagTxIFTAmount)
	}
	if !common.IsHexAddress(flagTxIFTTo) {
		return errors.Errorf("invalid --to address %q", flagTxIFTTo)
	}

	backend, key, chainID, err := dialIFTChain(cmd.Context())
	if err != nil {
		return err
	}

	contract, err := ift.NewContract(common.HexToAddress(flagTxIFTAddress), backend)
	if err != nil {
		return err
	}

	opts, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	if err != nil {
		return err
	}
	opts.Context = cmd.Context()

	tx, err := contract.Mint(opts, common.HexToAddress(flagTxIFTTo), amount)
	if err != nil {
		return errors.Wrap(err, "mint")
	}

	receipt, err := bind.WaitMined(cmd.Context(), backend, tx)
	if err != nil {
		return errors.Wrap(err, "mint: wait mined")
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return errors.Errorf("mint: transaction %s reverted", tx.Hash())
	}

	return printTxHash(tx.Hash().Hex())
}

func txIFTSend(cmd *cobra.Command, _ []string) error {
	clientID, receiver := flagTxIFTClientID, flagTxIFTTo
	amount := parseIFTAmount(flagTxIFTAmount)
	if amount == nil {
		return errors.Errorf("invalid --amount %q", flagTxIFTAmount)
	}

	backend, key, chainID, err := dialIFTChain(cmd.Context())
	if err != nil {
		return err
	}

	contract, err := ift.NewContract(common.HexToAddress(flagTxIFTAddress), backend)
	if err != nil {
		return err
	}

	opts, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	if err != nil {
		return err
	}
	opts.Context = cmd.Context()

	if flagTxIFTTimeout <= 0 {
		return errors.Errorf("invalid --timeout %s: must be positive", flagTxIFTTimeout)
	}
	timeout := uint64(time.Now().Add(flagTxIFTTimeout).Unix())

	tx, err := contract.IftTransfer(opts, clientID, receiver, amount, timeout)
	if err != nil {
		return errors.Wrapf(err, "iftTransfer %q", clientID)
	}

	receipt, err := bind.WaitMined(cmd.Context(), backend, tx)
	if err != nil {
		return errors.Wrapf(err, "iftTransfer %q: wait mined", clientID)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return errors.Errorf("iftTransfer %q: transaction %s reverted", clientID, tx.Hash())
	}

	return printTxHash(tx.Hash().Hex())
}

func printTxHash(txHash string) error {
	return config.PrintJSON(map[string]string{"txHash": txHash})
}

func parseIFTAmount(s string) *big.Int {
	amount, ok := new(big.Int).SetString(s, 10)
	if !ok || amount.Sign() < 0 {
		return nil
	}
	return amount
}

// dialIFTChain resolves --chain's RPC and --from's signer from config, and
// dials the chain.
func dialIFTChain(ctx context.Context) (*ethclient.Client, *ecdsa.PrivateKey, *big.Int, error) {
	cfg, err := setupHomeWithConfig()
	if err != nil {
		return nil, nil, nil, err
	}

	chain, ok := cfg.Chain(flagTxIFTChain)
	if !ok {
		return nil, nil, nil, errors.Errorf("chain %q not declared in config", flagTxIFTChain)
	}
	if chain.Type() != config.ChainTypeEVM {
		return nil, nil, nil, errors.Errorf("chain %q is not an EVM chain", flagTxIFTChain)
	}

	keyHex, err := deployerKeyHex(cfg, flagTxIFTFrom)
	if err != nil {
		return nil, nil, nil, err
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(keyHex, "0x"))
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "signer key")
	}

	backend, err := ethclient.DialContext(ctx, chain.EVM.RPC)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "dial %s", chain.EVM.RPC)
	}

	chainID, err := backend.ChainID(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "query chain id")
	}

	return backend, key, chainID, nil
}
