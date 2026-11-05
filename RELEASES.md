<!-- SPDX-License-Identifier: Apache-2.0 -->

# Releases

This page covers how to release the IBC CLI and how to verify a release.

Pushing a `cli/vX.Y.Z` tag runs the [release workflow](./.github/workflows/release.yml). The workflow builds the CLI, signs it, and creates a draft [GitHub release](https://github.com/cosmos/ibc/releases) with the files. It also publishes the signed image `ghcr.io/cosmos/ibc:cli-vX.Y.Z`. The tag starts with `cli/` so that Go resolves the `github.com/cosmos/ibc/cli` module at that version. File names and the image tag use `cli-` instead, because they cannot contain `/`. The [shared release actions](./.github/actions/README.md) explain the SBOMs, attestations, and signatures it produces.

## Cut a release

1. Create a `cli/vX.Y.Z` tag on a commit on `main` or a `release/*` branch. Push it with `git`:

   ```sh
   git tag -s cli/v0.2.0 -m cli/v0.2.0 origin/main
   git push origin cli/v0.2.0
   ```

   Or create it in the GitHub UI while publishing a release, with the notes and **Set as a pre-release** filled in. The UI creates the tag only on publish, so the release is live before its files are signed and before its image exists. The workflow attaches the signed files to the published release a few minutes later. Steps 3 and 4 are then already done.

2. Wait for the workflow to attach the signed files. For a tag pushed with `git`, it creates a draft release only after every file is verified and the image is signed. If a job fails, re-run the failed jobs within 7 days, while the run still keeps its build artifacts. After that, re-run all jobs. If the release is already published with some of its files, delete them first. A full re-run stops for a published release that has files.
3. Write the release notes in the draft. Leave **Set as a pre-release** as it is. The workflow sets it from the tag on every run: checked for a suffix such as `-rc.1`, unchecked otherwise.
4. Publish the release.

The workflow runs from the tagged commit. A release branch therefore needs the current `.github/workflows/release.yml` and `.github/actions`. For the same reason, the workflow's branch check only catches mistakes.

The tag ruleset decides who can release. It must cover `cli/v*` tags and restrict creating, updating, and deleting them. In this repository, the "Restrict tag creation" ruleset covers all tags. Only the Maintain and Admin repository roles and organization admins can bypass it.

## Verify a release

The commands below need the GitHub CLI and cosign v3 or later, the version the workflow signs with. Replace `<tag>` with the release tag, such as `cli/v0.2.0`, and `vX.Y.Z` with its version, such as `v0.2.0`.

### Verify the files

Download the release:

```sh
gh release download <tag> --repo cosmos/ibc --dir ibc-cli-vX.Y.Z
```

In the `ibc-cli-vX.Y.Z` directory, check the signature on `checksums.txt`:

```sh
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity https://github.com/cosmos/ibc/.github/workflows/release.yml@refs/tags/<tag> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The two `--certificate` flags accept only a signature from this repository's release workflow, run for `<tag>`.

Check that every file matches `checksums.txt`, which proves it is unchanged since signing. On macOS, use `shasum -a 256 -c checksums.txt` instead.

```sh
sha256sum -c checksums.txt
```

Check the build provenance of an archive. The attestation covers the `.tar.gz` file, not the binary inside it, so check the archive before you extract it.

```sh
gh attestation verify ibc-cli-vX.Y.Z-linux-amd64.tar.gz --repo cosmos/ibc \
  --signer-workflow cosmos/ibc/.github/workflows/release.yml \
  --source-ref refs/tags/<tag>
```

`--signer-workflow` and `--source-ref` accept only an attestation from the release workflow, run for `<tag>`.

Two flags change what this command checks:

- `--predicate-type https://spdx.dev/Document/v<version>` checks the SBOM attestation instead of the build provenance. `<version>` is the SPDX version the SBOM declares. `jq -r .spdxVersion ibc-cli-vX.Y.Z-linux-amd64.spdx.json` prints it as `SPDX-<version>`.
- `--bundle ibc-cli-vX.Y.Z.intoto.jsonl` checks against the attestation file attached to the release, without GitHub's attestations API.

After you extract an archive, `ibc --version` prints the release tag, the commit it was built from, and the pinned ibc-contracts and ibc-go versions and commits.

### Verify the image

Check the signature, with the same `--certificate` flags as for `checksums.txt`:

```sh
cosign verify ghcr.io/cosmos/ibc:cli-vX.Y.Z \
  --certificate-identity https://github.com/cosmos/ibc/.github/workflows/release.yml@refs/tags/<tag> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Check the build provenance:

```sh
gh attestation verify oci://ghcr.io/cosmos/ibc:cli-vX.Y.Z --repo cosmos/ibc \
  --signer-workflow cosmos/ibc/.github/workflows/release.yml \
  --source-ref refs/tags/<tag>
```

The image tag points at one image per platform. Each platform image is a separate build, so each has its own SBOM attestation. List the platform digests with `docker buildx imagetools inspect ghcr.io/cosmos/ibc:cli-vX.Y.Z`. Then run the command above on `oci://ghcr.io/cosmos/ibc@<digest>` with the `--predicate-type` flag.

The `main` and `branch-<branch>` images from the [IBC CLI build workflow](./.github/workflows/ibc-cli-build.yml) are unsigned development builds.
