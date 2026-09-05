<!-- SPDX-License-Identifier: Apache-2.0 -->

# ConnectRPC Guide for AI Agents

Development flow (commands run from the repository root):

1. For RPC schema changes, edit raw protos in `proto/cli/*` and run `make -C cli codegen-proto`.
2. Implement and test ConnectRPC handlers in `attestor_handler.go` or `relayer_handler.go`.
   Handler-only changes do not require schema edits or code generation.
3. Add missing `AttestorService`/`RelayerService` methods if needed and run
   `make -C cli codegen-mocks` when the service interface changes.
   - Use idiomatic Go: `ctx` first.
   - If a service method needs more than 3 args, use a DTO struct.
4. Keep `server/` as an adapter only:
   - Convert ConnectRPC/proto types to service DTOs.
   - Call the service interface.
   - Map domain errors to `connect.Code*`.
