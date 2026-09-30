# syntax=docker/dockerfile:1

# --- Frontend ---------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Backend ----------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/server ./cmd/server \
 && mkdir -p /out/data

# --- Laufzeit ---------------------------------------------------------------
# Keine RUN-Befehle in dieser Stufe: so baut buildx arm64 ohne QEMU-Emulation.
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/mrcdlm/dnsdeck" \
      org.opencontainers.image.description="Self-hosted dynamic DNS for Cloudflare with tunnel monitoring" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/server /app/server
COPY --from=build --chown=65532:65532 /out/data /data
USER nonroot:nonroot
ENV DATA_DIR=/data PORT=8080
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/app/server", "healthcheck"]
ENTRYPOINT ["/app/server"]
