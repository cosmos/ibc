package chain

import "math/big"

func GenesisPrefund() *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)
}
