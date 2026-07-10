package chain

import "math/big"

// GenesisPrefund is the balance, in base units (wei / astake), that every managed launcher writing a
// genesis grants its funded accounts: 10^30, astronomically more than any test moves, so funding is
// never the cause of a failure. One value across families keeps the magnitude from drifting per
// launcher.
func GenesisPrefund() *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)
}
