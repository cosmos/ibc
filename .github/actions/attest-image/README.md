<!-- SPDX-License-Identifier: Apache-2.0 -->

# attest-image

Call this action right after pushing an image. Pass the digest the push reported, because a tag can be moved to another image in the meantime. The [shared release actions](../README.md) page explains what it produces.

## Platform image

Push each platform image by digest, without BuildKit's own provenance, then attest it with its SBOM:

```yaml
jobs:
  image:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
      id-token: write
      attestations: write
      artifact-metadata: write
    steps:
      # Log in to the registry first.
      - id: push
        uses: docker/build-push-action@<commit-sha> # v7
        with:
          context: .
          platforms: linux/amd64
          outputs: type=image,name=ghcr.io/cosmos/ibc,push-by-digest=true,name-canonical=true,push=true
          provenance: false
      - uses: cosmos/ibc/.github/actions/attest-image@<commit-sha> # main
        with:
          image: ghcr.io/cosmos/ibc
          digest: ${{ steps.push.outputs.digest }}
```

`provenance: false` keeps the pushed digest pointing at the image itself. With BuildKit's provenance, it would point at an index wrapping the image.

## Multi-platform index

After combining the platform images into an index, attest and sign the index:

```yaml
      - uses: cosmos/ibc/.github/actions/attest-image@<commit-sha> # main
        with:
          image: ghcr.io/cosmos/ibc
          digest: ${{ steps.index.outputs.digest }}
          sbom: "false"
          sign: "true"
          recursive: "true"
```

- `sbom: "false"` skips the SBOM, because each platform image already has one.
- `recursive: "true"` also signs the platform images the index lists.

Create and tag the index as the `Publish manifest` and `Tag signed image` steps of cosmos/ibc's [release workflow](../../workflows/release.yml) do. They tag the index only after this action signs it, so the tag never points at an unsigned image.

## Single-platform image

For an image with one platform, call the action once with `sign: "true"`. It attests the image with its SBOM and signs it.

[`action.yml`](./action.yml) lists every input.
