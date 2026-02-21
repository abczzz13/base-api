# syntax=docker/dockerfile:1.7

ARG TARGETOS
ARG TARGETARCH
ARG GO_IMAGE=golang:1.26.0-alpine
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian12:nonroot

FROM ${GO_IMAGE} AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

WORKDIR /src

ENV CGO_ENABLED=0 \
    GOWORK=off

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build \
    -trimpath \
    -mod=readonly \
    -buildvcs=false \
    -ldflags="-s -w -X github.com/abczzz13/base-api/internal/version.buildVersion=${VERSION}" \
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
    org.opencontainers.image.created="${BUILD_TIME}"

COPY --from=builder /out/base-api /base-api

EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/base-api"]
