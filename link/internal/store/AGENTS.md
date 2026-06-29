# Database Development Guide for AI Agents

This project support Sqlite and Postgres, takes their similarities, differences and syntax into account.

## I. Creating migrations

To create a migration, run the script from the project's root (./link)

```bash
go run scripts/migratenew.go relay-submissions
✔︎ Created link/internal/store/migrations/sqlite/001-relay-submissions.sql
✔︎ Created link/internal/store/migrations/postgres/001-relay-submissions.sql
```


## II. Query creation, code generation

For types & queries generation, `sqlc` tool is used.

When creating `sqlc` SQL queries in `queries/*`, use "macros" feature to build unified files that can be
used both for sqlite and postgres. Example:

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

Generate Go bindings using `make codegen-sql` which outputs code generated files to 
`repository/postgres` and `repository/sqlite`

### III. Unified repository layer

`TODO`
