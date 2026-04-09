# Multi-stage build for the VC Platform
FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o vcplatform ./cmd/server/

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /build/vcplatform .
COPY --from=builder /build/config ./config
EXPOSE 8080
ENTRYPOINT ["./vcplatform"]
CMD ["-addr", ":8080", "-config", "config/default.json"]
