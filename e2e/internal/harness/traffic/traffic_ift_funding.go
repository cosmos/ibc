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

// Coins returns n whole tokens (n * 10^18 base units) of an 18-decimal token.
func Coins(n int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(n), big.NewInt(weiPerEther))
}

// FundWallets pays every wallet tokenEach of the route's source IFT, out of
// the batch shim's IFT balance (the shim is minted the initial supply at
// deployment), and weiEach native coin (none if nil), on the source Chain.
// The route's sender first tops its own balance up to cover every wallet's
// native value through the Chain's funding capability, so the source Chain
// must be managed, then submits one payable fund transaction per 64
// wallets. A failed chunk stops funding, leaving earlier chunks paid.
func (i *IFT) FundWallets(ctx context.Context, wallets []common.Address, tokenEach, weiEach *big.Int) error {
	tokenEach, err := validAmount(tokenEach)
	if err != nil {
		return err
	}
	if weiEach == nil {
		weiEach = new(big.Int)
	}
	if len(wallets) == 0 {
		return nil
	}

	funding, err := i.source.chain.Funding()
	if err != nil {
		return fmt.Errorf("traffic: fund wallets on route %q: %w", i.routeID, err)
	}
	// The sender keeps its documented minimum after paying every wallet.
	minimum := new(big.Int).Mul(weiEach, big.NewInt(int64(len(wallets))))
	minimum.Add(minimum, RequiredSignerBalance())
	if err := funding.EnsureEOABalance(ctx, i.sender.Address(), minimum); err != nil {
		return fmt.Errorf("traffic: fund wallet funder %s on route %q: %w", i.sender.Address(), i.routeID, err)
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
