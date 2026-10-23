<!-- SPDX-License-Identifier: Apache-2.0 -->

# Store

`store.Store` is the single entrypoint for other packages that rely on DB.

- From `cli/`, create paired migrations with `go run scripts/migratenew.go <name>`. Edit both
  `migrations/sqlite/` and `migrations/postgres/`; keep their behavior equivalent while using each
  dialect's syntax.
- For persistence or schema changes, test against both backends from `cli/`. PostgreSQL tests are
  skipped unless `TEST_POSTGRES=true` and Docker is available. Report any backend you could not
  verify.
- `queries/` is shared by both dialects: write portable SQL with `sqlc.arg` and `sqlc.narg`, then
  run `make codegen-sql` from `cli/`.
- Add public persistence methods to the `Repository` interface in `store.go`, not to `Store`.
- Return domain structs from `store.go`, not generated sqlc row types.
