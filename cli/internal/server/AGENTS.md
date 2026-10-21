<!-- SPDX-License-Identifier: Apache-2.0 -->

# ConnectRPC adapters

Keep this package as a transport adapter: convert ConnectRPC/proto types to service DTOs, call the
service interface, and map domain errors to `connect.Code*`.

When an RPC schema changes, update `proto/cli/` and run `make codegen` from `cli/` before updating
the handler.
