<!-- SPDX-License-Identifier: Apache-2.0 -->

# ConnectRPC adapters

Keep this package as a transport adapter: convert ConnectRPC/proto types to service DTOs, call the
service interface, and map domain errors to `connect.Code*`.

When an RPC schema changes, update `proto/cli/`, run `make -C cli codegen-proto` from the repository
root, and include the updated bindings in `cli/api/v2/` before updating the handler.
