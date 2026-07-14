# Store guidelines for AI Agents

`store.Store` is the single entrypoint for other packages that rely on DB.

## I. Creating migrations

To create a migration, run the script from the project's root (./link)

```bash
go run scripts/migratenew.go add-packet-index
✔︎ Created /path/to/ibc/link/internal/store/migrations/sqlite/002-add-packet-index.sql
✔︎ Created /path/to/ibc/link/internal/store/migrations/postgres/002-add-packet-index.sql
```

The script prints absolute paths and assigns the next migration number automatically.

Edit both `migrations/sqlite/*.sql` and `migrations/postgres/*.sql`; keep behavior identical, syntax dialect-specific.

## II. Query creation, code generation

For types & queries generation, `sqlc` tool is used.

When creating `sqlc` SQL queries in `queries/*`, use "macros" to build unified files that 
can be used both for sqlite and postgres. Example:

```sql
-- name: GetAuthorByName :one
SELECT *
FROM authors
WHERE lower(name) = sqlc.arg(name);
-- "sqlc.arg" is a macro, "sqlc.narg" is similar, but makes a param nullable
-- macros docs: https://docs.sqlc.dev/en/latest/reference/macros.html

-- >>> EXPANDS TO >>>

-- name: GetAuthorByName :one
SELECT *
FROM authors
WHERE lower(name) = ?;
```

- (re)generate bindings with `make codegen-sql`.
- Don't edit generated files in `repository/sqlite` or `repository/postgres`.

### III. Unified repository layer

After ensuring that the new migration is in place and the sqlc models and queries have been generated:

- Add public persistence methods to `Repository` interface in `store.go`. Use idiomatic Go
- Implement every method in both `store_sqlite.go` and `store_postgres.go`.
- Return domain structs from `store.go`, not generated sqlc row types.
- Implement basic input validation. if method arg is a struct, implement `.Validate() error` for it
