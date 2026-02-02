# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o proxy-harold ./cmd/server

# Run stage
FROM alpine:latest

WORKDIR /app

# Add certificates for HTTPS fetching
RUN apk add --no-cache ca-certificates

# Copy binary from builder
COPY --from=builder /app/proxy-harold .

# Create directory for cache
RUN mkdir -p /app/cache_data

# Environment variables
ENV PORT=8888
ENV CACHE_DIR=/app/cache_data
ENV CACHE_TTL=1h
ENV RATE_LIMIT=100
ENV RATE_BURST=200

EXPOSE 8888

# Run the application
ENTRYPOINT ["./proxy-harold"]
