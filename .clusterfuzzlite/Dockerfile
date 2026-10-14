# SPDX-License-Identifier: Apache-2.0

FROM gcr.io/oss-fuzz-base/base-builder-go:v1@sha256:1dbadefbb28c6bfd796c64d9c2115977deb8f19494533cc5a3b724c3441df23d
COPY . $SRC/ibc
WORKDIR $SRC/ibc/cli
COPY .clusterfuzzlite/build.sh $SRC/
