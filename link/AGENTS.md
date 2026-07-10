# IBC Link Development Guide for AI Agents

- Use `make lint-fix` to auto-format and lint code before finishing work.
- Don't add verbose comments. Be concise.
- Black-box e2e lives in `harness/` and `e2e/` (separate Go modules with their own guides). When a
  real implementation of a wire command lands here, flip its entry in the SUT routing table
  (`harness/ibclink/runner.go`) and delete the stub piece it replaces — `make test-e2e` must stay
  green.
