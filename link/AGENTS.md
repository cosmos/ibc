# IBC Link Development Guide for AI Agents

- Use `make lint-fix` to auto-format and lint code before finishing work.
- Don't add verbose comments. Be concise.
- Repository-wide black-box e2e lives in `../e2e`, with its harness in
  `../e2e/internal/harness` as a separate Go module. When a
  real implementation satisfies an e2e wire contract, change the relevant `Driver` method's
  binary selection in `../e2e/internal/harness/ibclink/runner.go` (or the long-lived command in
  `daemon.go`) and delete the stub piece it replaces — `make test-e2e` from the repository root
  must stay green.
