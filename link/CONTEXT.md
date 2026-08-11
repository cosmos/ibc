<!-- SPDX-License-Identifier: Apache-2.0 -->

# IBC Link Environment

The IBC Link environment context describes the on-chain protocol topology and off-chain actors that a test declares and obtains as a ready Environment.

## Language

**Environment Spec**:
A declaration of the Chains, IBC Instances, IBC Connections, Attestors, and Relayers a test requires. It describes desired resources rather than their ready state.

**Environment**:
The realized, ready set of resources declared by an Environment Spec. It is the lifecycle boundary through which a test accesses those resources.

**Realization**:
The process by which an Environment Spec becomes an Environment, satisfying each declaration with a newly created or already-existing resource.

**Chain**:
A blockchain network that hosts zero or more IBC Instances.

**IBC Instance**:
An installation of the IBC protocol hosted on a Chain. An IBC Instance may be newly created or already exist.
_Avoid_: Deployment

**IBC Client**:
One end of an IBC Connection, hosted by an IBC Instance and tracking the counterparty. Its counterparty client identifier points to the reciprocal IBC Client.

**IBC Connection**:
The IBC protocol relationship between two IBC Instances, constituted by a reciprocal pair of IBC Clients.
_Avoid_: Route as a synonym for an IBC Connection

**Attestor**:
An independent logical actor configured to serve a particular IBC Client. Its identity is independent of process or endpoint placement.

**Relayer**:
An independent logical actor that relays protocol messages for one or more IBC Connections. Its identity is independent of process or endpoint placement, and multiple Relayers may serve the same IBC Connection.

**Relayer Route**:
A directed configuration belonging to a Relayer when it needs to carry messages in one direction over an IBC Connection. It is not an independent Environment resource and does not replace the Connection.

**Test Application**:
A test-only application deployed on a Chain to originate protocol traffic or expose observable state. It is explicit test setup, not an Environment resource.

**Test Application Binding**:
A typed, non-owning test-side handle to the source and destination contracts used by one application on a directed synthetic route. It hides application ABI and RPC mechanics but does not own lifecycle, relayer state, or cross-resource orchestration.
