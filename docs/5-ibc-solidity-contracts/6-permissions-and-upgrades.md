---
title: "Permissions and upgrades"
description: "Which roles gate the IBC-solidity contracts, who holds them on a deployment, which contracts sit behind proxies, and which inputs are fixed at construction."
---

On a deployment of the [IBC-solidity contracts](1-overview.md), who may call the gated functions and who may replace the code depends on the contract.

- **The [router](2-ics26-router.md), IBCMulticall, and [ICS27GMP](3-ics27-gmp-and-accounts.md)** share a single OpenZeppelin access manager, which holds the roles and decides which addresses may call the functions those roles gate. Its admin may also replace the code behind them.
- **Light clients** are each bound at construction to one router, the only address that may drive them, and that router's access manager decides who may [pause them](#permissions-on-a-light-client). No light client has an upgrade path: changing one means deploying a replacement that takes over its client ID.
- **An IFT** answers to its own owner or authority, which may also replace its code.

Everything else about a deployment was fixed when each contract was constructed.

## The access manager and its roles

The manager is OpenZeppelin's [`AccessManager`](https://docs.openzeppelin.com/contracts/5.x/access-control#access-management). Three contracts defer to it: the [router](2-ics26-router.md), IBCMulticall, and [ICS27GMP](3-ics27-gmp-and-accounts.md). IBCMulticall batches a relayer's deliveries to the router, so the router sees it as the caller of each one. Each contract is initialized with the manager's address as its `authority`, and the IBC CLI passes the same manager to all three. A gated function on any of them carries the `restricted` modifier, which asks the manager whether this caller may call this selector here. So changing who may call what requires a change to the manager.

All other contracts carry their own access control.

| Contract | Who decides who may call it | Source |
|----------|-----------------------------|--------|
| [`ICS26Router`](2-ics26-router.md#roles-and-permissions) | The access manager | [ICS26Router.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/ICS26Router.sol) |
| `IBCMulticall` | The access manager | [IBCMulticall.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/utils/IBCMulticall.sol) |
| [`ICS27GMP`](3-ics27-gmp-and-accounts.md#roles-pausing-and-upgrades) | The access manager | [ICS27GMP.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/ICS27GMP.sol) |
| [`ICS27Account`](3-ics27-gmp-and-accounts.md#the-account-contracts-functions) | ICS27GMP, the only outside address that may drive an account. The manager's admin still reaches the account code, through `ICS27GMP.upgradeAccountTo` | [ICS27Account.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/utils/ICS27Account.sol) |
| [`AttestationLightClient`](5-attestation-light-client.md#permissions-and-pausing) | The router it is bound to, fixed at construction. Pausing answers to that router's access manager | [AttestationLightClient.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/light-clients/attestation/AttestationLightClient.sol) |
| [`BesuLightClient`](7-besu-light-client.md#permissions-and-pausing) | The router it is bound to, fixed at construction. Pausing answers to that router's access manager | [BesuLightClient.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/light-clients/besu/BesuLightClient.sol) |
| [`IFTOwnable`](4-ift-contracts.md#roles-and-permissions) | Its owner | [IFTOwnable.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/utils/IFTOwnable.sol) |
| [`IFTAccessManaged`](4-ift-contracts.md#roles-and-permissions) | An authority its issuer passes at initialization, which may or may not be this manager | [IFTAccessManaged.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/utils/IFTAccessManaged.sol) |

`IBCRolesLib` names the roles and the selector set each one is written to gate. A deployment decides which of them it assigns. The ibc-contracts deployment script, [`DeployAccessManagerWithRoles`](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/scripts/deployments/DeployAccessManagerWithRoles.sol), assigns every role below to its selectors. The deployment the IBC CLI runs assigns only `PUBLIC_ROLE`.

| Role | ID | What it gates |
|------|----|---------------|
| `ADMIN_ROLE` | 0 | The proxy and beacon upgrade selectors, `migrateClient`, and every role grant by default |
| `RELAYER_ROLE` | 1 | `recvPacket`, `ackPacket`, `timeoutPacket`, `updateClient`, `verifyMembership`, and `verifyNonMembership` on the [router](2-ics26-router.md), and `multicall` on IBCMulticall. The ibc-contracts deployment script also grants this role to IBCMulticall, so its batches reach the router |
| `PAUSER_ROLE` | 2 | `pause` on the router and ICS27GMP. Every light client bound to the router checks this role itself |
| `UNPAUSER_ROLE` | 3 | `unpause` on the router and ICS27GMP. Every light client bound to the router checks this role itself |
| `ID_CUSTOMIZER_ROLE` | 6 | The custom-identifier forms of `addIBCApp` and `addClient` |
| `PUBLIC_ROLE` | `type(uint64).max` | Whatever selectors a deployment assigns to it, and every address holds it |

An admin makes both kinds of change, and both are calls on the manager rather than on the contracts it governs.

- **To give an address a role**, call `grantRole(roleId, account, executionDelay)`. A nonzero delay means that address cannot call a gated function directly: it must `schedule` the call, wait out the delay, then `execute` it. Zero lets it call immediately. To take it back, call `revokeRole(roleId, account)`.
- **To change which functions a role gates**, call `setTargetFunctionRole(target, selectors, roleId)`. The target is an argument, so the map is per contract: `ADMIN_ROLE` gates `migrateClient` and the upgrade selectors on the [router](2-ics26-router.md), and a different set on [ICS27GMP](3-ics27-gmp-and-accounts.md).

[Grant, change, and revoke a role](../6-ibc-cli/9-grant-change-and-revoke-a-role.md) shows both changes on the tutorial's deployment.

## What a deployment assigns

The IBC CLI deploys the contract set, and `ibc deploy core` makes two assignments out of the role map above, with no flag and no branch around either call. It binds `recvPacket`, `ackPacket`, `timeoutPacket`, `updateClient`, `multicall`, and `submitMisbehaviour` on the router to `PUBLIC_ROLE`. Only the first four carry `restricted`, so the router's `multicall` and `submitMisbehaviour` are open to any caller with or without that binding. It also binds `multicall` on IBCMulticall to `PUBLIC_ROLE`. It never grants `RELAYER_ROLE`, not even to IBCMulticall. So on a chain the IBC CLI brings up, any address may relay, and the relayer role gates nothing.

The relayer sends each batch through `IBCMulticall`, so the router sees `IBCMulticall` as the caller, and that is the relayer address it reports to IBC applications. Because the router's relay functions are public, `IBCMulticall` needs no role of its own to call them. The contract's own documentation describes the locked-down alternative: `multicall` mapped to `RELAYER_ROLE`, and `RELAYER_ROLE` granted to `IBCMulticall` so it can call the router.

The manager treats a function it was never told about as gated by `ADMIN_ROLE`. `ibc deploy core` assigns nothing else, so every other `restricted` function answers to the admin alone. That includes `pause` and `unpause` on the router and ICS27GMP, the router's `verifyMembership` and `verifyNonMembership`, and every form of `addIBCApp` and `addClient`. The ibc-contracts deployment script assigns the forms in which the router picks the identifier to `PUBLIC_ROLE`, so any address may call them. On a chain the IBC CLI brings up, only the admin may register an application or a client.

`ibc deploy core` grants no one `PAUSER_ROLE` or `UNPAUSER_ROLE`. A light client checks those roles directly rather than through a selector assignment, and the manager does not count its admin as holding them, so no address can pause a light client on a chain the IBC CLI brings up until an admin grants `PAUSER_ROLE`.

The manager's constructor takes its first admin as an argument. `ibc deploy core` passes the deployer key and leaves it there, and the driver's own comment calls role hardening a follow-up. The rest of the manager's surface is in [OpenZeppelin's reference](https://docs.openzeppelin.com/contracts/5.x/api/access#AccessManager).

Visit the [tutorial](../6-ibc-cli/2-tutorial-deploy-ibc-and-send-a-token.md) for a hands-on guide on deploying these contracts with IBC CLI. 

## Permissions on a light client

A light client sits outside the access manager's selector map, but it defers to the router it was constructed with. That address is immutable and must be nonzero. Only it may call `updateClient`, `verifyMembership`, `verifyNonMembership`, and `misbehaviour`, so proofs reach a client only through the router's own gates. `ibc deploy client` binds each client to this chain's router.

Pausing is the one decision a client takes from the access manager. `pause` and `unpause` look the caller up in the bound router's current authority, under `PAUSER_ROLE` and `UNPAUSER_ROLE`, and reject a holder with an execution delay, because the client cannot consume a scheduled call. Granting or revoking either role on the manager takes effect at once on every client bound to that router. A paused client rejects updates and packet proofs, so packets on its connections stop. Whether it can still be frozen depends on the client type: the [Besu light client](7-besu-light-client.md#permissions-and-pausing) accepts misbehaviour evidence while paused, but the [attestation light client](5-attestation-light-client.md) freezes only in `updateClient`, which pausing blocks.

## Configuration fixed at construction

A few inputs are set when a contract is constructed and have no setter afterwards, so they are the values to get right before deploying. Changing one later means deploying a new contract in place of the old.

| Contract | Fixed at construction |
|----------|-----------------------|
| [`AttestationLightClient`](5-attestation-light-client.md) | The attestor addresses, the signature threshold, an initial height with its timestamp, and the router it is bound to |
| [`BesuLightClient`](7-besu-light-client.md#constructor-and-configuration) | The counterparty router it proves, the trusting period (at most 30 days), the maximum clock drift (at most 12 seconds), the trust level, an initial trusted consensus state, and the router it is bound to |
| `CosmosIFTSendCallConstructor` | The mint message type URL, the counterparty denom, and the interchain account address |

## Proxies and upgrade authority

A core contract is deployed as two contracts: logic that holds the code, and an ERC1967 proxy that holds the address and the state. An upgrade points the proxy at new logic, so the address and the state survive it.

| Contract | Proxy | Who may change the logic |
|----------|-------|--------------------------|
| `ICS26Router` | UUPS behind an ERC1967 proxy | The access manager's admin, through `upgradeToAndCall` |
| `IBCMulticall` | UUPS behind an ERC1967 proxy | The access manager's admin, through `upgradeToAndCall` |
| `ICS27GMP` | UUPS behind an ERC1967 proxy | The access manager's admin, through `upgradeToAndCall` |
| `ICS27Account` | Beacon proxy, one per account identifier | The access manager's admin, through `ICS27GMP.upgradeAccountTo` |
| [`IFTOwnable`](4-ift-contracts.md) | UUPS behind an ERC1967 proxy | Its owner |
| [`IFTAccessManaged`](4-ift-contracts.md) | UUPS behind an ERC1967 proxy | Its own authority |

All three access-managed contracts mark `_authorizeUpgrade` as `restricted`. The role map registers the upgrade selectors under the admin role, so no other role reaches them by default. The accounts move as one group. Their logic sits behind a single beacon that ICS27GMP owns, so one `upgradeAccountTo` call changes every account.

A contract deployed without a proxy holds its settings in constructor-set state, so it changes by replacement. Every light client is in that group, along with the Cosmos send-call constructor. The AccessManager is not proxied either, so its code changes by replacement, but its role grants and its selector map stay changeable by an admin.

A UUPS upgrade is one call on the proxy, `upgradeToAndCall(newImplementation, data)`. The proxy has no upgrade code. Its current logic runs the call:

1. `_authorizeUpgrade` checks the caller with the access manager.
2. The call reverts unless the new logic's `proxiableUUID()` returns the ERC1967 implementation slot, where the proxy stores its logic's address. This rejects logic that does not implement UUPS. Such logic has no upgrade function, so the proxy could never upgrade again.
3. The call writes the new address to that slot and emits `Upgraded`.
4. If `data` is nonempty, the proxy executes it with the new logic's code, on the proxy's storage. A release puts its migration call in `data`.

The upgrade call does not check the storage layout. The new logic reads the proxy's state by slot position. Inserting, removing, or reordering fields in any of a contract's storage structs moves existing fields to the wrong slots. Appending a field is safe only where nothing is stored after the struct: at the end of a contract's top-level storage struct, or in a struct stored as a mapping value. In a struct stored in an array, or nested ahead of other fields, an appended field moves everything stored after it.

The access manager stores each function-role assignment under the proxy's address and a selector, so assignments survive an upgrade. Only `ADMIN_ROLE` holders may call a selector with no assignment. So after an upgrade, only admins can call a function if the release newly marks it `restricted` or changes its signature, and so its selector. If `recvPacket`'s signature changed, relaying would stop until an admin binds the new selector to the relaying role with `setTargetFunctionRole`.

[Upgrade an IBC contract](../6-ibc-cli/8-upgrade-the-router-contract.md) shows a router upgrade on the tutorial's deployment.

## Replacing a light client

A client changes by replacement, and the replacement takes over the client ID. The router's `migrateClient` function takes a client ID, its counterparty info, and the address of a newly deployed client contract. It then points the registry entry at that contract. Counterparties and applications go on using the same identifier.

Only an admin may call `migrateClient`. The role map registers its selector under the admin role, so no other role reaches it by default.

Migration is also how a frozen client is restarted. A frozen client keeps reverting its update and verification calls, so what returns the connection to service is a newly deployed client, migrated in under the same client ID. IBC-solidity's security model asks for that restart, and expects the admin role to sit with a timelocked Security Council.

Each client type decides what freezes it. The [attestation light client](5-attestation-light-client.md) freezes on conflicting attested timestamps, and the [Besu light client](7-besu-light-client.md#what-freezes-the-client) on the conflicts its page lists. Nothing in either clears that flag.

The [tutorial](../6-ibc-cli/2-tutorial-deploy-ibc-and-send-a-token.md) deploys the access manager and leaves the deployer as the only address granted a role. [Grant, change, and revoke a role](../6-ibc-cli/9-grant-change-and-revoke-a-role.md) assigns roles on that deployment.
