# Self-contained two-stage build for the inji-preauth-proxy sidecar.
# Uses its own go.mod — the binary only depends on the Go stdlib so we
# don't drag in the parent verifiably-go module's dependencies.

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 go build -o /out/inji-preauth-proxy .

FROM alpine:3.21
RUN adduser -D -u 10001 proxy
COPY --from=builder /out/inji-preauth-proxy /usr/local/bin/inji-preauth-proxy
USER proxy
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/inji-preauth-proxy"]
