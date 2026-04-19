# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.25-alpine AS builder
WORKDIR /src

# Go sources + module graph.
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY backend ./backend
COPY internal ./internal
COPY vctypes ./vctypes

# Static binary — no cgo, no dynamic libs, safe to drop into distroless.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/verifiably ./cmd/server

# ---- Runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=builder /out/verifiably /app/verifiably
COPY templates ./templates
COPY static ./static
COPY config ./config

ENV VERIFIABLY_ADDR=:8080 \
    VERIFIABLY_ADAPTER=mock \
    VERIFIABLY_BACKENDS_FILE=/app/config/backends.json \
    VERIFIABLY_AUTH_PROVIDERS_FILE=/app/config/auth-providers.json \
    VERIFIABLY_LOCALES_DIR=/app/locales
EXPOSE 8080
# /app/locales is writable via a volume mount in production so translation
# cache persists across container recreates.
VOLUME ["/app/locales"]
ENTRYPOINT ["/app/verifiably"]
