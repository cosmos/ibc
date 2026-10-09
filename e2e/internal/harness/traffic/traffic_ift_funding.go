// SPDX-License-Identifier: Apache-2.0

package traffic

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/cosmos/ibc/gen/go/solidity-abi/iftbatchtransfershim"
)

// fundChunk caps the wallets per fund transaction so one funding transaction
// stays well inside a block's gas limit.
const fundChunk = 64

// FundWallets pays every wallet tokenEach of the route's source IFT, out of
// the batch shim's IFT balance (the shim is minted the initial supply at
// deployment), and weiEach native coin (none if nil), on the source Chain. It
// submits one payable fund transaction per 64 wallets from the route's
// sender, which must hold weiEach for every wallet plus gas. A failed chunk
// stops funding, leaving earlier chunks paid. Since the tokens leave the
// shim, do not fund wallets between a SendBatch and its VerifyBurned or
// VerifyState, which compare the shim's balance with the one before sending.
func (i *IFT) FundWallets(ctx context.Context, wallets []common.Address, tokenEach, weiEach *big.Int) error {
	tokenEach, err := validAmount(tokenEach)
	if err != nil {
		return err
	}
	if weiEach == nil {
		weiEach = new(big.Int)
	}
	for start := 0; start < len(wallets); start += fundChunk {
		chunk := wallets[start:min(start+fundChunk, len(wallets))]
		recipients := make([]iftbatchtransfershim.IFTBatchTransferShimRecipient, len(chunk))
		for k, wallet := range chunk {
			recipients[k] = iftbatchtransfershim.IFTBatchTransferShimRecipient{Account: wallet, Amount: tokenEach}
		}
		data, err := calldata(func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return iftBatchShimTransactor.Fund(opts, i.sourceIFT, recipients, weiEach)
		})
		if err != nil {
			return fmt.Errorf("traffic: pack IFT batch shim fund on route %q: %w", i.routeID, err)
		}
		value := new(big.Int).Mul(weiEach, big.NewInt(int64(len(chunk))))
		if _, err := i.source.evm.BroadcastTx(ctx, i.sender, &i.batcher, data, value); err != nil {
			return fmt.Errorf("traffic: fund wallets %d:%d on route %q: %w", start, start+len(chunk), i.routeID, err)
		}
	}
	return nil
}
