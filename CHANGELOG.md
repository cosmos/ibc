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

### Improvements

- [\#803](https://github.com/cosmos/ibc/pull/803) Changed UpgradeState enums to match the opening handshake enum style.
