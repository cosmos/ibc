package store

import "context"

type transactionStore interface {
	withTx(ctx context.Context, fn func(repo Repository) error) error
}
