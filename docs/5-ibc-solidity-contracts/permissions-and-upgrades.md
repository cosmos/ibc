---
title: "Permissions and upgrades"
description: "Which roles gate the IBC-solidity contracts, who holds them on a deployment, which contracts sit behind proxies, and which inputs are fixed at construction."
---

On a deployment of the [IBC-solidity contracts](/ibc-solidity-contracts/overview), who may call the gated functions and who may replace the code depends on the contract.

- **The [router](/ibc-solidity-contracts/ics26-router) and [ICS27GMP](/ibc-solidity-contracts/ics27-gmp-and-accounts)** share a single OpenZeppelin access manager, which holds the roles and decides which addresses may call the functions those roles gate. Its admin may also replace the code behind them.
- **Light clients** sit outside that manager. The attestation light client answers to [a role manager fixed when it was constructed](#permissions-on-a-light-client). No light client has an upgrade path: changing one means deploying a replacement that takes over its client ID.
- **An IFT** answers to its own owner or authority, which may also replace its code.

Everything else about a deployment was fixed when each contract was constructed.

## The access manager and its roles

The manager is OpenZeppelin's [`AccessManager`](https://docs.openzeppelin.com/contracts/5.x/access-control#access-management). Two contracts defer to it: the [router](/ibc-solidity-contracts/ics26-router) and [ICS27GMP](/ibc-solidity-contracts/ics27-gmp-and-accounts). Each is initialized with the manager's address as its `authority`, and `ibc deploy core` passes the same manager to both. A gated function on either one carries the `restricted` modifier, which asks the manager whether this caller may call this selector here. So changing who may call what requires a change to the manager.

All other contracts carry their own access control.

| Contract | Who decides who may call it | Source |
|----------|-----------------------------|--------|
| [`ICS26Router`](/ibc-solidity-contracts/ics26-router#roles-and-permissions) | The access manager | [ICS26Router.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/ICS26Router.sol) |
| [`ICS27GMP`](/ibc-solidity-contracts/ics27-gmp-and-accounts#roles-pausing-and-upgrades) | The access manager | [ICS27GMP.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/ICS27GMP.sol) |
| [`ICS27Account`](/ibc-solidity-contracts/ics27-gmp-and-accounts#the-account-contracts-functions) | ICS27GMP, the only outside address that may drive an account. The manager's admin still reaches the account code, through `ICS27GMP.upgradeAccountTo` | [ICS27Account.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/utils/ICS27Account.sol) |
| [`AttestationLightClient`](/ibc-solidity-contracts/attestation-light-client#roles-and-permissions) | Its own roles, fixed at construction | [AttestationLightClient.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/light-clients/attestation/AttestationLightClient.sol) |
| [`IFTOwnable`](/ibc-solidity-contracts/ift-contracts#roles-and-permissions) | Its owner | [IFTOwnable.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/utils/IFTOwnable.sol) |
| [`IFTAccessManaged`](/ibc-solidity-contracts/ift-contracts#roles-and-permissions) | An authority its issuer passes at initialization, which may or may not be this manager | [IFTAccessManaged.sol](https://github.com/cosmos/ibc-contracts/blob/main/ibc-solidity/contracts/utils/IFTAccessManaged.sol) |

`IBCRolesLib` names the roles and the selector set each one is written to gate. A deployment decides which of them it assigns, and the one IBC CLI runs assigns a single entry.

| Role | ID | What it gates |
|------|----|---------------|
| `ADMIN_ROLE` | 0 | The proxy and beacon upgrade selectors, `migrateClient`, and every role grant by default |
| `RELAYER_ROLE` | 1 | `recvPacket`, `ackPacket`, `timeoutPacket`, and `updateClient` on the [router](/ibc-solidity-contracts/ics26-router) |
| `PAUSER_ROLE` | 2 | `pause` on ICS27GMP |
| `UNPAUSER_ROLE` | 3 | `unpause` on ICS27GMP |
| `ID_CUSTOMIZER_ROLE` | 6 | The custom-identifier forms of `addIBCApp` and `addClient` |
| `PUBLIC_ROLE` | `type(uint64).max` | Whatever selectors a deployment assigns to it, and every address holds it |

An admin makes both kinds of change, and both are calls on the manager rather than on the contracts it governs.

- **To give an address a role**, call `grantRole(roleId, account, executionDelay)`. A nonzero delay means that address cannot call a gated function directly: it must `schedule` the call, wait out the delay, then `execute` it. Zero lets it call immediately. To take it back, call `revokeRole(roleId, account)`.
- **To change which functions a role gates**, call `setTargetFunctionRole(target, selectors, roleId)`. The target is an argument, so the map is per contract: `ADMIN_ROLE` gates `migrateClient` and the upgrade selectors on the [router](/ibc-solidity-contracts/ics26-router), and a different set on [ICS27GMP](/ibc-solidity-contracts/ics27-gmp-and-accounts).

## What a deployment assigns

IBC Link deploys the contract set, and `ibc deploy core` makes one assignment out of the role map above. It binds `recvPacket`, `ackPacket`, `timeoutPacket`, `updateClient`, `multicall`, and `submitMisbehaviour` on the router to `PUBLIC_ROLE`, with no flag and no branch around the call. It never grants `RELAYER_ROLE`. So on a chain IBC Link brings up, any address may relay, and the relayer role gates nothing.

The manager's constructor takes its first admin as an argument. `ibc deploy core` passes the deployer key and leaves it there, and the driver's own comment calls role hardening a follow-up. The rest of the manager's surface is in [OpenZeppelin's reference](https://docs.openzeppelin.com/contracts/5.x/api/access#AccessManager).

## Permissions on a light client

A light client sits outside the access manager, so the client's own constructor decides who may submit proofs to it. For the attestation light client that is the fifth constructor argument, `roleManager`. A nonzero address there receives both the client's admin role and its proof-submitter role. Whether that address can grant or revoke them afterwards depends on what it is. `ibc deploy client` passes the [router](/ibc-solidity-contracts/ics26-router), which exposes no way to call `grantRole`, so on a client IBC Link deployed those roles cannot be moved without upgrading the router. Deploy with a zero role manager and the submitter role goes to the zero address instead, which leaves proof submission open to every caller.

<Warning>
Deploying an attestation light client by hand with a zero role manager leaves proof submission open for the life of that client. A client with no admin cannot be closed later.
</Warning>

## Configuration fixed at construction

A few inputs are set when a contract is constructed and have no setter afterwards, so they are the values to get right before deploying. Changing one later means deploying a new contract in place of the old.

| Contract | Fixed at construction |
|----------|-----------------------|
| [`AttestationLightClient`](/ibc-solidity-contracts/attestation-light-client) | The attestor addresses, the signature threshold, and an initial height with its timestamp |
| `CosmosIFTSendCallConstructor` | The mint message type URL, the counterparty denom, and the interchain account address |

## Proxies and upgrade authority

A core contract is deployed as two contracts: logic that holds the code, and an ERC1967 proxy that holds the address and the state. An upgrade points the proxy at new logic, so the address and the state survive it.

| Contract | Proxy | Who may change the logic |
|----------|-------|--------------------------|
| `ICS26Router` | UUPS behind an ERC1967 proxy | The access manager's admin, through `upgradeToAndCall` |
| `ICS27GMP` | UUPS behind an ERC1967 proxy | The access manager's admin, through `upgradeToAndCall` |
| `ICS27Account` | Beacon proxy, one per account identifier | The access manager's admin, through `ICS27GMP.upgradeAccountTo` |
| [`IFTOwnable`](/ibc-solidity-contracts/ift-contracts) | UUPS behind an ERC1967 proxy | Its owner |
| [`IFTAccessManaged`](/ibc-solidity-contracts/ift-contracts) | UUPS behind an ERC1967 proxy | Its own authority |

Both core contracts mark `_authorizeUpgrade` as `restricted`. The role map registers the upgrade selectors under the admin role, so no other role reaches them by default. The accounts move as one group. Their logic sits behind a single beacon that ICS27GMP owns, so one `upgradeAccountTo` call changes every account.

A contract deployed without a proxy holds its settings in constructor-set state, so it changes by replacement. Every light client is in that group, along with the Cosmos send-call constructor. The AccessManager is not proxied either, so its code changes by replacement, but its role grants and its selector map stay changeable by an admin.

## Replacing a light client

A client changes by replacement, and the replacement takes over the client ID. The router's `migrateClient` function takes a client ID, its counterparty info, and the address of a newly deployed client contract. It then points the registry entry at that contract. Counterparties and applications go on using the same identifier.

Only an admin may call `migrateClient`. The role map registers its selector under the admin role, so no other role reaches it by default.

Migration is also how a frozen client is restarted. A frozen client keeps reverting its update and verification calls, so what returns the connection to service is a newly deployed client, migrated in under the same client ID. IBC-solidity's security model asks for that restart, and expects the admin role to sit with a timelocked Security Council.

Each client type decides what freezes it. The [attestation light client](/ibc-solidity-contracts/attestation-light-client) freezes on conflicting attested timestamps, and nothing in it clears that flag.
