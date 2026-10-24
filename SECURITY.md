<!-- SPDX-License-Identifier: Apache-2.0 -->

# Security Policy

## Reporting a vulnerability

Do not open a public issue, pull request, or discussion for a suspected vulnerability. Public disclosure before a fix is available puts adopters and user funds at risk.

In-scope vulnerabilities are reported through the [Cosmos bug bounty program on Immunefi](https://immunefi.com/bug-bounty/cosmos/). In scope means a bug that gives an attacker a capability they should not have on a correctly configured production chain or relayer: minting, stealing, or locking funds; forging or replaying packets; bypassing light client verification; or halting a chain through IBC. Include affected versions, impact, the conditions required, and an end-to-end proof of concept.

Issues outside that scope go to public GitHub issues through the normal process, or, where borderline, to the private IBC security channel ([GitHub Security Advisories](https://github.com/cosmos/ibc/security/advisories/new) or the restricted IBC security list).

### Triage and timelines

Reports are triaged by a dedicated IBC triage, run by the Cosmos Labs Foundations team together with the external IBC maintainers, twice a week. Severity follows the Cosmos security classification matrix. Fix targets by severity: critical five weeks, high four, medium six, low eight.

### Fix and disclosure

For low and medium issues, and some high, the fix is a backported public patch across the maintained release lines. For critical issues the fix is prepared in a private GitHub Security Advisory fork; affected chains are pre-announced with severity and date but no detail, then sent the patch privately under embargo before the public release; and the release, advisory, and CVE are published with credit to the reporter. If an issue is being exploited or leaks, it is published immediately.

### Downstream distribution list

Security contacts of production IBC chains, major relayer operators, and other IBC implementations. To join: a named contact, proof of production use, signed embargo terms, and KYC. Members are contacted only once a fix is ready. Leaks lead to removal.

### AI-assisted and automated reports

AI-assisted and automated reports must be verified by a human, concise, in plain text, and state affected files, versions, and verifiable impact rather than theoretical consequences, with a tested reproducer. A human owns the submission and AI use is disclosed. Where the finding is in scope, with funds or a live chain at risk, report it through Immunefi rather than publicly, since public disclosure before chains upgrade is itself the exploit. Lower-impact AI findings follow the public path.

### Rewards

In-scope reports are eligible for a cash reward by severity through the Immunefi program, paid after the public fix.

### Supported versions

Security fixes target the supported release lines listed in [RELEASES.md](RELEASES.md).

### Artifact signing

Releases are signed with Sigstore so adopters can verify provenance.

### Ownership

The security process is owned jointly by the Cosmos Labs Foundations team and the IBC maintainers under LFDT. External maintainers are onboarded to the triage and the bounty program.

### Scope note

This policy covers the IBC monorepo, including specs, implementations, and the relayer and attestor infrastructure.
