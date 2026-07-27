# IBC Link Development Guide for AI Agents

- Use `make lint-fix` to auto-format and lint code before finishing work.
- Don't add verbose comments. Be concise.
- Repository-wide black-box e2e lives in `../e2e`, with its harness in
  `../e2e/internal/harness` as a separate Go module. The real Link relayer submits packets through
  ICS26Router with empty proofs, accepted by the harness's permissive dummy light client
  (`go-abigen` from solidity-ibc-eureka is a pinned dependency for this). Keep
  `make test-e2e` green when changing that transport contract.
