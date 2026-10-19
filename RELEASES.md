<!-- SPDX-License-Identifier: Apache-2.0 -->

# Releases

## Signed release assets

Pushing a `cli-v*` tag runs the [release workflow](./.github/workflows/release.yml). It builds the IBC CLI for each platform from the tagged commit, generates an SPDX SBOM for each binary, attests their build provenance, signs a `checksums.txt` over them with cosign, and attaches everything to a draft release for the tag. The release notes get a section explaining how to verify the assets.

| Asset | Purpose |
| --- | --- |
| `ibc-<version>-<platform>.tar.gz` | Binary with `LICENSE` and `NOTICE` (`darwin-arm64`, `linux-amd64`, `linux-arm64`) |
| `ibc-<version>-<platform>.spdx.json` | SPDX SBOM of each binary |
| `checksums.txt` | SHA-256 of the archives and the SBOMs |
| `checksums.txt.sigstore.json` | Cosign signature over `checksums.txt` |
| `ibc-<version>.intoto.jsonl` | Provenance and SBOM attestation bundles, for offline `gh attestation verify --bundle` |

The same release workflow runs in [cosmos/ibc-go](https://github.com/cosmos/ibc-go) and [cosmos/ibc-contracts](https://github.com/cosmos/ibc-contracts); only the tag patterns and the build differ.

### Release procedure

1. Tag the release commit and push the tag (e.g. `git tag cli-v0.2.0 origin/main && git push origin cli-v0.2.0`).
2. Wait for the [release workflow](https://github.com/cosmos/ibc/actions/workflows/release.yml) to create the draft release in [GitHub](https://github.com/cosmos/ibc/releases) with the signed assets attached.
3. Edit the draft and write the release notes above the generated "Verifying the release" section.
4. Check the `Set as a pre-release` checkbox if needed. The workflow already checks it for tags with a pre-release suffix such as `-rc.1`.
5. Publish the release.

Push the tag with `git` before publishing the release. A release drafted from the GitHub UI only creates its tag when it is published, so its assets would be attached after publishing. The workflow runs from the tagged commit, so a release branch must contain the current `.github/workflows/release.yml`.

### Container images

The [release workflow](./.github/workflows/release.yml) publishes `ghcr.io/cosmos/ibc:cli-vX.Y.Z` from the release binaries, attests its build provenance and per-platform SBOMs, and signs it with cosign. The `ghcr.io/cosmos/ibc:main` image is not signed. Verify a release image with:

```sh
cosign verify ghcr.io/cosmos/ibc:cli-vX.Y.Z \
  --certificate-identity-regexp '^https://github.com/cosmos/ibc/.github/workflows/release.yml@' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
gh attestation verify oci://ghcr.io/cosmos/ibc:cli-vX.Y.Z --repo cosmos/ibc
```

Image SBOMs are attached to the platform digests, not the multi-platform tag. To check one, pass the platform digest with `--predicate-type https://spdx.dev/Document/v2.3`.
