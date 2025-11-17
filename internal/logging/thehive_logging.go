package logging

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// LoggingTransport wraps an http.RoundTripper to add structured logging
type LoggingTransport struct {
	Transport http.RoundTripper
}

// RoundTrip implements http.RoundTripper interface
func (t *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	// Log the outgoing request
	t.logRequest(req, start)

	// Execute the request
	resp, err := t.transport().RoundTrip(req)

	duration := time.Since(start)

	// Log the response
	t.logResponse(req, resp, err, duration)

	return resp, err
}

func (t *LoggingTransport) transport() http.RoundTripper {
	if t.Transport != nil {
		return t.Transport
	}
	return http.DefaultTransport
}

func (t *LoggingTransport) logRequest(req *http.Request, start time.Time) {
	// Read and restore request body for logging
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	slog.Info("HTTP request started",
		slog.String("method", req.Method),
		slog.String("url", req.URL.String()),
		slog.String("user_agent", req.UserAgent()),
		slog.Int("content_length", int(req.ContentLength)),
		slog.Time("timestamp", start),
		slog.String("request_id", t.getRequestID(req)),
	)
}

func (t *LoggingTransport) logResponse(req *http.Request, resp *http.Response, err error, duration time.Duration) {
	if err != nil {
		slog.Error("HTTP request failed",
			slog.String("method", req.Method),
			slog.String("url", req.URL.String()),
			slog.String("error", err.Error()),
			slog.Duration("duration", duration),
			slog.String("request_id", t.getRequestID(req)),
		)
		return
	}

	level := slog.LevelInfo
	if resp.StatusCode >= 400 {
		level = slog.LevelWarn
	}
	if resp.StatusCode >= 500 {
		level = slog.LevelError
	}

	slog.Log(context.Background(), level, "HTTP request completed",
		slog.String("method", req.Method),
		slog.String("url", req.URL.String()),
		slog.Int("status_code", resp.StatusCode),
		slog.String("status", resp.Status),
		slog.Duration("duration", duration),
		slog.Int64("content_length", resp.ContentLength),
		slog.String("content_type", resp.Header.Get("Content-Type")),
		slog.String("request_id", t.getRequestID(req)),
	)
}

func (t *LoggingTransport) getRequestID(req *http.Request) string {
	if id := req.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	return fmt.Sprintf("%p", req)
}
