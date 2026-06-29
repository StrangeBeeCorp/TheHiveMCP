# syntax=docker/dockerfile:1.4

# Stage 1: Build the Go binary
FROM golang:1.26.4-alpine AS builder

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

# Build the standalone healthcheck probe. It is a separate static binary so the
# distroless runtime needs no shell or HTTP client for its HEALTHCHECK.
RUN go build -ldflags="-s -w" -o /healthcheck ./cmd/healthcheck/main.go

# Stage 2: Runtime image
FROM gcr.io/distroless/static-debian12
LABEL io.modelcontextprotocol.server.name="io.github.StrangeBeeCorp/TheHiveMCP"

# Copy the server and the standalone healthcheck probe. No shell or HTTP client
# is added, so the image stays fully distroless.
COPY --from=builder /binary /app/server
COPY --from=builder /healthcheck /app/healthcheck

# Port the HTTP server listens on. The Go code has no default — it reads MCP_PORT
# from the environment — so the image owns the value here. Both the server and
# the healthcheck probe read MCP_PORT / MCP_SERVER_ENDPOINT from this same ENV,
# so the probe target can never drift from where the server listens. Change the
# port in this single place, or override at build time with --build-arg MCP_PORT.
ARG MCP_PORT=8082
ENV MCP_BIND_HOST=0.0.0.0 \
    MCP_PORT=${MCP_PORT}

# Non-root user (Distroless already uses nonroot by default)
USER nonroot:nonroot

WORKDIR /app

# Liveness probe: the standalone binary GETs the MCP endpoint, reads only the
# status line, and exits without draining the SSE stream. Exec form needs no
# shell; the probe resolves the port/path itself from the ENV above.
HEALTHCHECK --interval=10s --timeout=5s --retries=30 --start-period=10s \
    CMD ["/app/healthcheck"]

CMD ["/app/server"]
