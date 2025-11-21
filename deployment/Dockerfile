# syntax=docker/dockerfile:1.4

# Stage 1: Build the Go binary
FROM golang:1.24.9-alpine AS builder

# Set necessary environment variables
ARG BINARY=server
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /app

# Copy and download dependencies first for better cache utilization
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .
RUN go build -o /binary ./cmd/${BINARY}/main.go

# Stage 2: Runtime image
FROM gcr.io/distroless/static-debian12

# Copy built binary and config
COPY --from=builder /binary /app/server

# Non-root user (Distroless already uses nonroot by default)
USER nonroot:nonroot

WORKDIR /app
CMD ["/app/server"]
