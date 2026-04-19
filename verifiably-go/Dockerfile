# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.24-alpine AS builder
WORKDIR /src

# Only Go sources and go.mod are needed to produce the binary.
COPY go.mod ./
COPY cmd ./cmd
COPY backend ./backend
COPY internal ./internal
COPY vctypes ./vctypes

# Static binary — no cgo, no dynamic libs, safe to drop into scratch/distroless.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/verifiably ./cmd/server

# ---- Runtime stage ----
# distroless/static has no shell/package manager — just libc and CA certs.
# Runs as uid 65532 (nonroot) by default.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=builder /out/verifiably /app/verifiably
COPY templates ./templates
COPY static ./static

ENV VERIFIABLY_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/app/verifiably"]
