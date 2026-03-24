# ── Builder ──────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

ARG VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /relaybox ./cmd/server/

# ── Runtime ──────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S relaybox \
    && adduser -S relaybox -G relaybox

WORKDIR /app

COPY --from=builder /relaybox /app/relaybox

RUN mkdir -p /app/data/queue \
    && chown -R relaybox:relaybox /app/data

USER relaybox

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app/relaybox"]
CMD ["start", "--config", "/app/config.yaml"]
