<!-- SPDX-License-Identifier: Apache-2.0 -->

# Releases

## Release artifacts

| Tag | Artifact | Description |
| --- | --- | --- |
| `cli-vX.Y.Z` | `ibc-cli-vX.Y.Z-darwin-arm64.tar.gz` | IBC CLI binary for darwin/arm64, with `LICENSE` and `NOTICE` |
|  | `ibc-cli-vX.Y.Z-linux-amd64.tar.gz` | IBC CLI binary for linux/amd64, with `LICENSE` and `NOTICE` |
|  | `ibc-cli-vX.Y.Z-linux-arm64.tar.gz` | IBC CLI binary for linux/arm64, with `LICENSE` and `NOTICE` |
|  | `ibc-cli-vX.Y.Z-darwin-arm64.spdx.json` | SPDX SBOM of the darwin/arm64 binary |
|  | `ibc-cli-vX.Y.Z-linux-amd64.spdx.json` | SPDX SBOM of the linux/amd64 binary |
|  | `ibc-cli-vX.Y.Z-linux-arm64.spdx.json` | SPDX SBOM of the linux/arm64 binary |
|  | `checksums.txt` | SHA-256 checksums of the release files, the SBOMs and the attestation bundle |
|  | `checksums.txt.sigstore.json` | Cosign signature over `checksums.txt` |
|  | `ibc-cli-vX.Y.Z.intoto.jsonl` | Build provenance and SBOM attestations, for verification without the attestations API with `gh attestation verify --bundle` |
|  | `ghcr.io/cosmos/ibc:<tag>` | IBC CLI container image (linux/amd64, linux/arm64), signed with cosign and carrying provenance and SBOM attestations |

The files are attached to the tag's [GitHub release](https://github.com/cosmos/ibc/releases) and the image is published to [GHCR](https://github.com/cosmos/ibc/pkgs/container/ibc).

The image is also published, unsigned, as `ghcr.io/cosmos/ibc:main` on pushes to `main` that change the CLI.

See [Signed release assets](#signed-release-assets) for how to verify the artifacts.

## Signed release assets

Pushing a `cli-v*` tag runs the [release workflow](./.github/workflows/release.yml). It builds the IBC CLI for each platform from the tagged commit, generates an SPDX SBOM for each binary, attests their build provenance, signs a `checksums.txt` over them with cosign, and attaches everything to a draft release for the tag. It also publishes the signed [container image](#container-images) built from the same linux binaries.

`cli-v*` tags are reserved for IBC CLI releases. A `cli-vX.Y.Z` tag produces `ibc-cli-vX.Y.Z` assets and the "IBC CLI vX.Y.Z" title; the image is tagged with the pushed tag, e.g. `ghcr.io/cosmos/ibc:cli-vX.Y.Z`. Other tags, such as the 2019–2020 spec release candidates `v1.0.0-rc2` to `v1.0.0-rc6`, do not run the release workflow.


The attesting, signing and publishing steps are the [shared release actions](./.github/actions/README.md), which [cosmos/ibc-go](https://github.com/cosmos/ibc-go) and [cosmos/ibc-contracts](https://github.com/cosmos/ibc-contracts) use too; each repository only defines its tag patterns and its build. Here extra jobs build the container image from the linux archives and publish its multi-platform tag once the assets verify.

### Release procedure

1. Tag the release commit and push the tag (e.g. `git tag -s cli-v0.2.0 -m cli-v0.2.0 origin/main && git push origin cli-v0.2.0`).
2. Wait for the [release workflow](https://github.com/cosmos/ibc/actions/workflows/release.yml) to create the draft release in [GitHub](https://github.com/cosmos/ibc/releases) with the signed assets attached. Every asset is verified against its attestations before any image is pushed, and the draft is only created once the multi-platform image is tagged and signed, so a draft always comes with a signed image. If a job fails, re-run the failed jobs.
3. Edit the draft and write the release notes.
4. Check the `Set as a pre-release` checkbox if needed. The workflow already checks it for tags with a pre-release suffix such as `-rc.1`.
5. Publish the release.

Push the tag with `git` before publishing the release. A release drafted from the GitHub UI only creates its tag when it is published, so its assets would be attached after publishing. The workflow runs from the tagged commit, so a release branch must contain the current `.github/workflows/release.yml` and `.github/actions`.

The workflow only accepts a commit that is on `main` or a `release/*` branch whose ruleset requires a pull request with at least one approval. Add such a ruleset to a release branch before tagging from it; classic branch protection does not count. The check runs from the tagged commit, so it only guards against tagging an unreviewed commit by mistake. What limits who can get a commit signed is the tag ruleset, which must cover `refs/tags/cli-v*` and restrict creations, updates and deletions. In this repository the "Restrict tag creation" ruleset covers all tags, and only the Maintain and Admin repository roles and organization admins can bypass it, so only they can push a release tag.

### Verifying release files

Download the release files, where `<tag>` is the pushed tag:

```sh
gh release download <tag> --repo cosmos/ibc --dir <tag> && cd <tag>
```

Check the signature over `checksums.txt`, then check the files against it (on macOS, use `shasum -a 256 -c checksums.txt`):

```sh
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity https://github.com/cosmos/ibc/.github/workflows/release.yml@refs/tags/<tag> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum -c checksums.txt
```

Check the build provenance of an archive:

```sh
gh attestation verify ibc-cli-vX.Y.Z-linux-amd64.tar.gz --repo cosmos/ibc \
  --signer-workflow cosmos/ibc/.github/workflows/release.yml \
  --source-ref refs/tags/<tag>
```

Add `--predicate-type https://spdx.dev/Document/v2.3` to check its SBOM attestation instead. Add `--bundle ibc-cli-vX.Y.Z.intoto.jsonl` to verify against the attestation bundle attached to the release rather than the GitHub attestations API.

### Container images

The [release workflow](./.github/workflows/release.yml) publishes `ghcr.io/cosmos/ibc:<tag>` from the release binaries, attests its build provenance and per-platform SBOMs, and signs it with cosign. The `ghcr.io/cosmos/ibc:main` image, published by the [IBC CLI build workflow](./.github/workflows/ibc-cli-build.yml) on pushes to `main`, is not signed. The workflow pushes the index by digest, signs it, and only then tags it `<tag>`, so the release tag never points at an unsigned image. Verify a release image with:

```sh
cosign verify ghcr.io/cosmos/ibc:<tag> \
  --certificate-identity https://github.com/cosmos/ibc/.github/workflows/release.yml@refs/tags/<tag> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
gh attestation verify oci://ghcr.io/cosmos/ibc:<tag> --repo cosmos/ibc \
  --signer-workflow cosmos/ibc/.github/workflows/release.yml \
  --source-ref refs/tags/<tag>
```

Image SBOMs are attached to the platform digests, not the multi-platform tag. List the platform digests with `docker buildx imagetools inspect ghcr.io/cosmos/ibc:<tag>`, then pass one as `oci://ghcr.io/cosmos/ibc@<digest>` with `--predicate-type https://spdx.dev/Document/v2.3`.
