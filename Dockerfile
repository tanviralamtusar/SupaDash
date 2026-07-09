FROM golang:1.26.1-alpine AS builder

WORKDIR /app

# Download dependencies first so this layer is cached unless go.mod/go.sum change
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the whole main package (not just main.go) so all files compile in.
# Static binary, stripped for size.
ARG CACHE_BUST=1
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/supadash .

# Runtime stage
FROM alpine:latest

# docker-cli            → `docker stats/inspect/update` used by the collector/resource manager
# docker-cli-compose    → `docker compose` v2 subcommand used by the provisioner, brancher
#                         and edge-function deploys (the legacy `docker-compose` binary does
#                         NOT provide the `docker compose` subcommand these call)
RUN apk --no-cache add ca-certificates wget curl docker-cli docker-cli-compose && \
    addgroup -S supadash && adduser -S supadash -G supadash

WORKDIR /app

COPY --from=builder /app/supadash .
COPY --from=builder /app/.env.example .
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /app/templates ./templates

# Provisioned project stacks, edge-function sources and functions.env live here.
# Mount a volume at this path in production so they survive restarts.
VOLUME ["/app/projects"]

EXPOSE 8080

# /v1/health is unauthenticated. The MCP server is served on the same port at /mcp.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/v1/health || exit 1

CMD ["./supadash"]
