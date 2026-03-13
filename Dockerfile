# syntax=docker/dockerfile:1.7

ARG TARGETOS
ARG TARGETARCH
ARG GO_IMAGE=golang:1.26.1-alpine
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian12:nonroot

FROM ${GO_IMAGE} AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG GIT_BRANCH=unknown
ARG GIT_TAG=unknown
ARG GIT_TREE_STATE=unknown
ARG BUILD_TIME=unknown

WORKDIR /src

ENV CGO_ENABLED=0 \
    GOWORK=off

RUN apk add --no-cache ca-certificates

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build \
    -trimpath \
    -mod=vendor \
    -buildvcs=false \
    -ldflags="-s -w \
    -X github.com/abczzz13/base-api/internal/version.buildVersion=${VERSION} \
    -X github.com/abczzz13/base-api/internal/version.gitCommit=${GIT_COMMIT} \
    -X github.com/abczzz13/base-api/internal/version.gitBranch=${GIT_BRANCH} \
    -X github.com/abczzz13/base-api/internal/version.gitTag=${GIT_TAG} \
    -X github.com/abczzz13/base-api/internal/version.gitTreeState=${GIT_TREE_STATE} \
    -X github.com/abczzz13/base-api/internal/version.buildTime=${BUILD_TIME}" \
    -o /out/base-api \
    ./cmd/api

FROM ${RUNTIME_IMAGE} AS runner

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

LABEL org.opencontainers.image.source="https://github.com/abczzz13/base-api" \
    org.opencontainers.image.title="Base API" \
    org.opencontainers.image.description="Base API service" \
    org.opencontainers.image.version="${VERSION}" \
    org.opencontainers.image.revision="${GIT_COMMIT}" \
    org.opencontainers.image.created="${BUILD_TIME}" \
    org.opencontainers.image.licenses=MIT \
    org.opencontainers.image.authors="abczzz13"

COPY --from=builder /out/base-api /base-api

EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/base-api"]
