# syntax=docker/dockerfile:1.4

# Stage 1: Build the Go binary
FROM golang:1.24.11-alpine AS builder

# Set necessary environment variables
ARG BINARY=server
ARG TARGETARCH
ARG TARGETOS
ENV CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH}

WORKDIR /app

# Copy and download dependencies first for better cache utilization
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .

# Set build variables for version injection
ARG BUILD_DATE
ARG GIT_COMMIT
ARG VERSION

# Build with version information injected via ldflags
RUN go build \
    -ldflags="-s -w -X 'github.com/StrangeBeeCorp/TheHiveMCP/version.buildDate=${BUILD_DATE}' -X 'github.com/StrangeBeeCorp/TheHiveMCP/version.gitCommit=${GIT_COMMIT}' -X 'github.com/StrangeBeeCorp/TheHiveMCP/version.gitVersion=${VERSION}'" \
    -o /binary ./cmd/${BINARY}/main.go

# Stage 2: Runtime image
FROM gcr.io/distroless/static-debian12
LABEL io.modelcontextprotocol.server.name="io.github.StrangeBeeCorp/TheHiveMCP"

# Copy built binary and config
COPY --from=builder /binary /app/server

# Non-root user (Distroless already uses nonroot by default)
USER nonroot:nonroot

WORKDIR /app
CMD ["/app/server"]
