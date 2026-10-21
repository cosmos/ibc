<!-- SPDX-License-Identifier: Apache-2.0 -->

# Store

`store.Store` is the single entrypoint for other packages that rely on DB.

- From `cli/`, create paired migrations with `go run scripts/migratenew.go <name>`. Edit both
  `migrations/sqlite/` and `migrations/postgres/`; keep their behavior equivalent while using each
  dialect's syntax.
- Write shared queries in `queries/` with sqlc arguments such as `sqlc.arg` and `sqlc.narg`, then run
  `make codegen-sql` from `cli/`.
- Don't edit generated files in `repository/sqlite` or `repository/postgres`.
- Add public persistence methods to the `Repository` interface in `store.go`.
- Implement every method in both `store_sqlite.go` and `store_postgres.go`.
- Return domain structs from `store.go`, not generated sqlc row types.
