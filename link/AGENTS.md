# IBC Link Development Guide for AI Agents

- Use `make lint-fix` to auto-format and lint code before finishing work.
- Don't add verbose comments. Be concise.
- Repository-wide black-box e2e lives in `../e2e`, with its harness in
  `../e2e/internal/harness` as a separate Go module. The e2e transport is real IBC:
  `internal/ibcrelay` relays packets through ICS26Router with empty proofs, accepted by the
  harness's permissive dummy light client (`go-abigen` from solidity-ibc-eureka is a pinned
  dependency for this). When a real implementation satisfies an e2e transport contract, change
  the explicit handler selection in `cmd/ibc/main.go` and delete the ibcrelay piece it
  replaces — `make test-e2e` from the repository root must stay green.
