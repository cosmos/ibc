<!-- SPDX-License-Identifier: Apache-2.0 -->

# ConnectRPC adapters

Keep this package as a transport adapter: convert ConnectRPC/proto types to service DTOs, call the
service interface, and map domain errors to `connect.Code*`.

When an RPC schema changes, update `proto/cli/` and run `make codegen-proto` from `cli/` before
updating the handler. Run `make codegen-mocks` from `cli/` when an affected mocked interface changes.
