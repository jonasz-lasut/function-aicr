# syntax=docker/dockerfile:1

# go.mod is the source of truth for the Go version: CI passes the version it
# declares as GO_VERSION. A local build defaults to the latest Go 1.x image, and
# Go's toolchain selection still guarantees at least the version go.mod requires.
ARG GO_VERSION=1

# Setup the base environment.
FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION} AS base

WORKDIR /fn
ENV CGO_ENABLED=0

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Build the Function.
FROM base AS build
ARG TARGETOS
ARG TARGETARCH
RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags='-s -w' -o /function .

# Produce the Function image.
FROM gcr.io/distroless/static-debian12:nonroot AS image
WORKDIR /
COPY --from=build /function /function
EXPOSE 9443
USER nonroot:nonroot
ENTRYPOINT ["/function"]
