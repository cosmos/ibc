# syntax=docker/dockerfile:1

# SPDX-License-Identifier: Apache-2.0

# 1. Source image builder
FROM golang:1.26.4-alpine AS builder

ARG LDFLAGS="-s -w"

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="$LDFLAGS" -o /out/ibc ./cmd/ibc/...

# 2. Shared runtime between local docker and docker on CI
FROM alpine:3.24.1 AS runtime

ENV HOME=/home/ibc

RUN mkdir -p "$HOME" && chown 65532:65532 "$HOME"

WORKDIR $HOME

EXPOSE 3000

USER 65532:65532
ENTRYPOINT ["/opt/ibc"]

# 3. Used for CI docker image.
# Expects current dir to contain valid `ibc` binary.
FROM runtime AS target-artifact
COPY ibc /opt/ibc

# 4. Used for local docker image building.
FROM runtime AS target-builder
COPY --from=builder /out/ibc /opt/ibc
