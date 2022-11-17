<!--
Guiding Principles:

Changelogs are for humans, not machines.
There should be an entry for every single version.
The same types of changes should be grouped.
Versions and sections should be linkable.
The latest version comes first.
The release date of each version is displayed.
Mention whether you follow Semantic Versioning.

Usage:

Change log entries are to be added to the Unreleased section under the
appropriate stanza (see below). Each entry should ideally include a tag and
the Github issue reference in the following format:

* (<tag>) \#<issue-number> message

The issue numbers will later be link-ified during the release process so you do
not have to worry about including a link manually, but you can if you wish.

Types of changes (Stanzas):

"New Applications" for new applications.
"New Light Clients" for new light clients.
"Relayers" for changes to relayer protocol.
"Features" for new features to the core IBC stack
"Improvements" for changes in existing functionality.
"Deprecated" for soon-to-be removed features.
"Bug Fixes" for any bug fixes.
"API Breaking" for breaking APIs expected by implementation teams.
"State Machine Breaking" for any changes that result in a different AppState given same genesisState and txList.
"Protocol Breaking" for any changes that would result in an established channel/connection/client no longer being able to communicate with its original counterparty. Note that any changes that are Protocol-Breaking **must** be supported by a backwards-compatibility preserving upgrade protocol.
Ref: https://keepachangelog.com/en/1.0.0/
-->

# Changelog

## [Unreleased]

### API-Breaking

- [\#873](https://github.com/cosmos/ibc/pull/873) Defines the connection `Version` datatype and adjusts the signature of `getCompatibleVersions()` and `pickVersion()` accordingly
- [\#813](https://github.com/cosmos/ibc/pull/813) Breaks up `checkValidityAndUpdateState` into `verifyClientMessage` and `UpdateState`
- [\#813](https://github.com/cosmos/ibc/pull/813) Breaks up `checkMisbehaviourAndUpdateState` into `checkForMisbehaviour` and `UpdateStateOnMisbehaviour`
- [\#813](https://github.com/cosmos/ibc/pull/813) Removes `Header` and `Misbehaviour` interfaces for generic `ClientMessage` interface
- [\#813](https://github.com/cosmos/ibc/pull/813) Removes specific verify functions from ClientState interface in exchange for generic `verifyMembership` and `verifyNonMembership` methods
- [\#813](https://github.com/cosmos/ibc/pull/813) Adds `getTimeoutAtHeight` method to ClientState interface.

### Bug Fixes

- [\#808](https://github.com/cosmos/ibc/pull/808) Fix channel sequence paths in ICS4
- [\#863](https://github.com/cosmos/ibc/pull/863) Fix allowed range for `trustLevel`
- [\#878](https://github.com/cosmos/ibc/pull/878) Removes broken localhost client spec ICS9

### Improvements

- [\#802](https://github.com/cosmos/ibc/pull/802) Move `ClientState` to the `provableStore` and add `ClientState` validation in `connOpenTry` and `connOpenAck`
- [\#803](https://github.com/cosmos/ibc/pull/803) Changed UpgradeState enums to match the opening handshake enum style.
- [\#804](https://github.com/cosmos/ibc/pull/804) Increment upgrade sequence at the start of a new handshake rather than the end of a completed handshake
- [\#806](https://github.com/cosmos/ibc/pull/806) Adds previous version to UpgradeInit and UpgradeTry callback arguments
- [\#807](https://github.com/cosmos/ibc/pull/807) Upgrade keys will now prefix the channel path to align with the rest of ICS4 keys
- [\#842](https://github.com/cosmos/ibc/pull/842) Adds metadata field to FungibleTokenPacketData
- [\#844](https://github.com/cosmos/ibc/pull/844) Adds event emission in `recvPacket` when `packet.sequence < nextSequenceRecv` for ordered channels and when `packetRecepit != null` for unordered channels
- [\#845](https://github.com/cosmos/ibc/pull/845) Adds explanation about `onRecvPacket` callback returning an error in interchain accounts controller modules
