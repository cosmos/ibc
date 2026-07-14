# IBC Link Development Guide for AI Agents

- Use `make lint-fix` to auto-format and lint code before finishing work.
- Don't add verbose comments. Be concise.
- Repository-wide black-box e2e lives in `../e2e`, with its harness in
  `../e2e/internal/harness` as a separate Go module. When a
  real implementation satisfies an e2e transport contract, change the explicit handler selection
  in `cmd/ibc/main.go` and delete the stub piece it replaces — `make test-e2e` from the repository
  root must stay green.
