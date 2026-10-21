<!-- SPDX-License-Identifier: Apache-2.0 -->

# Releases

## Release artifacts

| Tag | Artifact | Description |
| --- | --- | --- |
| `vX.Y.Z` or `cli-vX.Y.Z` | `ibc-vX.Y.Z-darwin-arm64.tar.gz` | IBC CLI binary for darwin/arm64, with `LICENSE` and `NOTICE` |
|  | `ibc-vX.Y.Z-linux-amd64.tar.gz` | IBC CLI binary for linux/amd64, with `LICENSE` and `NOTICE` |
|  | `ibc-vX.Y.Z-linux-arm64.tar.gz` | IBC CLI binary for linux/arm64, with `LICENSE` and `NOTICE` |
|  | `ibc-vX.Y.Z-darwin-arm64.spdx.json` | SPDX SBOM of the darwin/arm64 binary |
|  | `ibc-vX.Y.Z-linux-amd64.spdx.json` | SPDX SBOM of the linux/amd64 binary |
|  | `ibc-vX.Y.Z-linux-arm64.spdx.json` | SPDX SBOM of the linux/arm64 binary |
|  | `checksums.txt` | SHA-256 checksums of the release files, the SBOMs and the attestation bundle |
|  | `checksums.txt.sigstore.json` | Cosign signature over `checksums.txt` |
|  | `ibc-vX.Y.Z.intoto.jsonl` | Build provenance and SBOM attestations, for verification without the attestations API with `gh attestation verify --bundle` |
|  | `ghcr.io/cosmos/ibc:<tag>` | IBC CLI container image (linux/amd64, linux/arm64), signed with cosign and carrying provenance and SBOM attestations |

The files are attached to the tag's [GitHub release](https://github.com/cosmos/ibc/releases) and the image is published to [GHCR](https://github.com/cosmos/ibc/pkgs/container/ibc).

The image is also published, unsigned, as `ghcr.io/cosmos/ibc:main` on pushes to `main` that change the CLI.

See [Signed release assets](#signed-release-assets) for how to verify the artifacts.

## Signed release assets

Pushing a `v*` or `cli-v*` tag runs the [release workflow](./.github/workflows/release.yml). It builds the IBC CLI for each platform from the tagged commit, generates an SPDX SBOM for each binary, attests their build provenance, signs a `checksums.txt` over them with cosign, and attaches everything to a draft release for the tag. It also publishes the signed [container image](#container-images) built from the same linux binaries. The release notes get a section explaining how to verify the assets.

`v*` and `cli-v*` tags are reserved for IBC CLI releases, and pushing either runs the release workflow. Both forms produce the same `ibc-vX.Y.Z` assets and "IBC CLI vX.Y.Z" title; the image is tagged with the pushed tag. The existing `v1.0.0-rc2` to `v1.0.0-rc6` tags are 2019–2020 spec release candidates; the spec is no longer released under `v*` tags.


The attesting, signing and publishing steps are the [shared release actions](./.github/actions/README.md), which [cosmos/ibc-go](https://github.com/cosmos/ibc-go) and [cosmos/ibc-contracts](https://github.com/cosmos/ibc-contracts) use too; each repository only defines its tag patterns and its build. Here the build also pushes the container image, and an extra job publishes its multi-platform tag.

### Release procedure

1. Tag the release commit and push the tag (e.g. `git tag -s v0.2.0 -m v0.2.0 origin/main && git push origin v0.2.0`).
2. Wait for the [release workflow](https://github.com/cosmos/ibc/actions/workflows/release.yml) to create the draft release in [GitHub](https://github.com/cosmos/ibc/releases) with the signed assets attached. The draft is only created once every build job and the signed multi-platform image have succeeded. If one fails, re-run the failed jobs.
3. Edit the draft and write the release notes above the generated "Verifying the release" section.
4. Check the `Set as a pre-release` checkbox if needed. The workflow already checks it for tags with a pre-release suffix such as `-rc.1`.
5. Publish the release.

Push the tag with `git` before publishing the release. A release drafted from the GitHub UI only creates its tag when it is published, so its assets would be attached after publishing. The workflow runs from the tagged commit, so a release branch must contain the current `.github/workflows/release.yml` and `.github/actions`.

To run the release again for an existing tag, for example when its run can no longer be re-run, start the workflow manually from the tag: choose the tag under "Use workflow from" on the [release workflow](https://github.com/cosmos/ibc/actions/workflows/release.yml) page, or run `gh workflow run release.yml --repo cosmos/ibc --ref v0.2.0`. The run is signed the same way as one started by the tag push. This only works for tags whose commit has the manual trigger, and only while the release is a draft: for a published release, the run pushes the image again and then refuses to replace the published assets.

To test the release workflow from a branch before merging it, run it from the branch and choose an existing release tag as the `tag` input, e.g. `gh workflow run release.yml --repo cosmos/ibc --ref my-branch -f tag=cli-v0.1.0`. The run builds the tagged source with the branch's workflow and actions. GitHub can't list tags in the input dynamically, so after each release add its tag to the `tag` options in [`release.yml`](./.github/workflows/release.yml). This dry run builds, attests and verifies the assets and signs `checksums.txt` as a release would, and builds the images without pushing them. It creates no release: download the `release-dry-run` artifact of the run to check the files and the generated "Verifying the release" notes. The attestations and the signature are real and logged publicly by Sigstore, but name the branch and its commit rather than the tag, so they don't verify as a release.

### Container images

The [release workflow](./.github/workflows/release.yml) publishes `ghcr.io/cosmos/ibc:<tag>` from the release binaries, attests its build provenance and per-platform SBOMs, and signs it with cosign. The `ghcr.io/cosmos/ibc:main` image, published by the [IBC CLI build workflow](./.github/workflows/ibc-cli-build.yml) on pushes to `main`, is not signed. Verify a release image with:

```sh
cosign verify ghcr.io/cosmos/ibc:<tag> \
  --certificate-identity https://github.com/cosmos/ibc/.github/workflows/release.yml@refs/tags/<tag> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
gh attestation verify oci://ghcr.io/cosmos/ibc:<tag> --repo cosmos/ibc \
  --signer-workflow cosmos/ibc/.github/workflows/release.yml \
  --source-ref refs/tags/<tag>
```

Image SBOMs are attached to the platform digests, not the multi-platform tag. To check one, pass the platform digest with `--predicate-type https://spdx.dev/Document/v2.3`.
