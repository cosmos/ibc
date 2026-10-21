<!-- SPDX-License-Identifier: Apache-2.0 -->

# Store

`store.Store` is the single entrypoint for other packages that rely on DB.

- From `cli/`, create paired migrations with `go run scripts/migratenew.go <name>`. Edit both
  `migrations/sqlite/` and `migrations/postgres/`; keep their behavior equivalent while using each
  dialect's syntax.
- For persistence or schema changes, test against both backends from `cli/`. PostgreSQL tests are
  skipped unless `TEST_POSTGRES=true` and Docker is available. Report any backend you could not
  verify.
- Write shared queries in `queries/` with sqlc arguments such as `sqlc.arg` and `sqlc.narg`, then run
  `make codegen-sql` from `cli/`.
- Don't edit generated files in `repository/sqlite` or `repository/postgres`.
- Add public persistence methods to the `Repository` interface in `store.go`, implemented in both
  `store_sqlite.go` and `store_postgres.go`.
- Return domain structs from `store.go`, not generated sqlc row types.
