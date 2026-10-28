<!-- SPDX-License-Identifier: Apache-2.0 -->

# Shared release actions

These composite actions sign the release artifacts of cosmos/ibc, [cosmos/ibc-go](https://github.com/cosmos/ibc-go), and [cosmos/ibc-contracts](https://github.com/cosmos/ibc-contracts), so anyone can check where an artifact came from and that it is unchanged. Each repository builds its artifacts in its own `release.yml`, then calls these actions.

- An **SBOM** (software bill of materials) lists the packages an artifact was built from, in [SPDX](https://spdx.dev) format. Use it to check a release for vulnerable dependencies.
- An **attestation** is a signed statement tied to an artifact's SHA-256 digest. Each artifact gets two: its build provenance, which names the workflow and commit that built it, and its SBOM.
- A **signature** made with [cosign](https://docs.sigstore.dev/cosign/) names the workflow that signed a file or image. Sigstore issues the workflow a short-lived certificate, so there is no signing key to protect.

| Action | What it does |
| --- | --- |
| [`attest-release-assets`](./attest-release-assets/README.md) | Generates an SBOM and attests a build job's release files. |
| [`publish-draft-release`](./publish-draft-release/README.md) | Verifies the files, signs a `checksums.txt` of their SHA-256 hashes, and attaches everything to a draft release. |
| [`attest-image`](./attest-image/README.md) | Attests a pushed container image, and can sign it. |

Each action's README has a minimal example. The [release workflow](../workflows/release.yml) shows the full job layout.

## Use and change the actions

- Pin the actions by full commit SHA, because a tag or branch can move. The `cosmos` organization's Actions policy must allow `cosmos/ibc` actions in the calling repository.
- Signatures name the calling repository's own `release.yml`, because these are composite actions rather than a reusable workflow.
- ibc-go and ibc-contracts pick up a change only when they bump the SHA, so keep inputs backwards compatible.
- Do not tag the actions with `cli-v*` tags. Pushing one runs the IBC CLI release.
- To test a change, push a tag such as `cli-v0.0.0-test.1` in a fork and verify the draft release. Then delete the draft, the tag, and any images it pushed.
