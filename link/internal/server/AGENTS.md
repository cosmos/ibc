<!-- SPDX-License-Identifier: Apache-2.0 -->

# ConnectRPC Guide for AI Agents

Development flow:
1. Update raw protos in `proto/link/*`.
2. Run `just link::codegen` (proto and mocks included).
3. Implement ConnectRPC handlers in `attestor_handler.go` or `relayer_handler.go`.
4. Add missing `AttestorService`/`RelayerService` methods if needed.
   - Use idiomatic Go: `ctx` first.
   - If a service method needs more than 3 args, use a DTO struct.
5. Keep `server/` as an adapter only:
   - Convert ConnectRPC/proto types to service DTOs.
   - Call the service interface.
   - Map domain errors to `connect.Code*`.
