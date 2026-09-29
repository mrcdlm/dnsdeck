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
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && mkdir -p /out/data

# --- Laufzeit ---------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /app/server
COPY --from=build --chown=65532:65532 /out/data /data
USER nonroot:nonroot
ENV DATA_DIR=/data PORT=8080
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/app/server", "healthcheck"]
ENTRYPOINT ["/app/server"]
