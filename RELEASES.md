<!-- SPDX-License-Identifier: Apache-2.0 -->

# Releases

IBC publishes releases so adopters can plan upgrades. The release rhythm suits the component: libraries and infrastructure release regularly, while the on-chain contracts release only on deliberate, reviewed changes rather than on a schedule.

IBC contains multiple independently evolving components. A new release may introduce changes to one or more components without affecting others. Release notes must clearly identify the affected components and describe the scope of changes.

## Versioning

Releases follow Semantic Versioning. Breaking changes bump the major version and are called out in the changelog and release notes.

## Cadence and branches

Development happens on main. Releases are cut from release tags, with a release branch per supported minor line where backports are needed. Each release has notes generated from the changelog and Conventional Commit history.

## Changelog

The changelog is generated from Conventional Commit titles at release time, so a well-formed pull request title is what matters rather than a hand-written entry. CI checks the title format.

## Supported versions

The currently supported release lines are listed here and referenced by [SECURITY.md](SECURITY.md). Security fixes target supported lines. Different components of the monorepo may maintain their own release lines.

| Release line | Status | Supported until |
| --- | --- | --- |
| [TBD] | Supported | [TBD] |

## Signing

Release artifacts are signed with Sigstore so adopters can verify provenance.

## Software Bill of Materials

Each released artifact publishes a Software Bill of Materials in a standard format, either CycloneDX or SPDX, so adopters can track components and dependencies. This supports adopters who must meet the EU Cyber Resilience Act, and is built toward ahead of the Act's full application.
