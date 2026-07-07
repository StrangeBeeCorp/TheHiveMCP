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

// Transport wraps an http.RoundTripper to add structured logging
type Transport struct {
	Transport http.RoundTripper
}

// RoundTrip implements http.RoundTripper interface
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	t.logRequest(req, start)

	resp, err := t.transport().RoundTrip(req)

	duration := time.Since(start)

	t.logResponse(req, resp, err, duration)

	if err != nil {
		return resp, fmt.Errorf("round trip %s %s: %w", req.Method, req.URL, err)
	}

	return resp, nil
}

func (t *Transport) transport() http.RoundTripper {
	if t.Transport != nil {
		return t.Transport
	}

	return http.DefaultTransport
}

func (t *Transport) logRequest(req *http.Request, start time.Time) {
	// Restore body after reading so the transport can still send it.
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

func (t *Transport) logResponse(req *http.Request, resp *http.Response, err error, duration time.Duration) {
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
	if resp.StatusCode >= http.StatusBadRequest {
		level = slog.LevelWarn
	}

	if resp.StatusCode >= http.StatusInternalServerError {
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

func (t *Transport) getRequestID(req *http.Request) string {
	if id := req.Header.Get("X-Request-ID"); id != "" {
		return id
	}

	return fmt.Sprintf("%p", req)
}
