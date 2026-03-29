FROM golang:1.26.1-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
ARG CACHE_BUST=1
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/supadash main.go

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates wget docker-cli docker-compose curl && \
    addgroup -S supadash && adduser -S supadash -G supadash

WORKDIR /app

COPY --from=builder /app/supadash .
COPY --from=builder /app/.env.example .
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /app/templates ./templates

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/v1/health || exit 1

CMD ["./supadash"]
