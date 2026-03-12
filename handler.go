package caddyreverseproxydump

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/gsmlg-dev/caddy-reverse-proxy-dump/internal/boundedbuffer"
)

func init() {
	caddy.RegisterModule(Handler{})
	httpcaddyfile.RegisterHandlerDirective("reverse_proxy_dump", parseCaddyfile)
	httpcaddyfile.RegisterDirectiveOrder("reverse_proxy_dump", httpcaddyfile.Before, "reverse_proxy")
}

// Handler implements the reverse_proxy_dump middleware.
type Handler struct {
	SinkType        string   `json:"sink_type,omitempty"`
	FilePath        string   `json:"file_path,omitempty"`
	MaxCaptureBytes int      `json:"max_capture_bytes,omitempty"`
	RedactHeaders   []string `json:"redact_headers,omitempty"`
	ContentTypes    []string `json:"content_types,omitempty"`

	sink   Sink
	logger *zap.Logger
}

// CaddyModule returns the Caddy module information.
func (Handler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.reverse_proxy_dump",
		New: func() caddy.Module { return new(Handler) },
	}
}

// Provision sets up the handler.
func (h *Handler) Provision(ctx caddy.Context) error {
	h.logger = ctx.Logger()

	if h.MaxCaptureBytes == 0 {
		h.MaxCaptureBytes = 1048576 // 1 MiB
	}
	if len(h.RedactHeaders) == 0 {
		h.RedactHeaders = defaultRedactHeaders
	}
	if h.SinkType == "" {
		h.SinkType = "console"
	}

	var err error
	switch h.SinkType {
	case "file":
		h.sink, err = NewJSONLSink(h.FilePath, h.logger)
		if err != nil {
			return fmt.Errorf("failed to open sink file: %w", err)
		}
	case "console":
		h.sink = NewConsoleSink(h.logger)
	default:
		return fmt.Errorf("unknown sink type: %s", h.SinkType)
	}

	return nil
}

// Validate ensures the handler configuration is valid.
func (h *Handler) Validate() error {
	if h.SinkType == "file" && h.FilePath == "" {
		return fmt.Errorf("file sink requires a file path")
	}
	return nil
}

// Cleanup drains the sink on shutdown.
func (h *Handler) Cleanup() error {
	if h.sink != nil {
		return h.sink.Close()
	}
	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	startTime := time.Now()

	// Source request ID
	requestID := h.getRequestID(r)

	// Capture request headers (redacted)
	reqHeaders := redactHeaders(r.Header, h.RedactHeaders)
	reqContentType := r.Header.Get("Content-Type")

	// Determine if we should capture request body based on content_types filter
	captureReqBody := h.shouldCaptureBody(reqContentType)

	// Tee request body into bounded buffer
	reqBuf := boundedbuffer.New(h.MaxCaptureBytes)
	if captureReqBody && r.Body != nil {
		r.Body = io.NopCloser(io.TeeReader(r.Body, reqBuf))
	}

	// Build request record
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	reqRecord := &RequestRecord{
		Method:        r.Method,
		Scheme:        scheme,
		Host:          r.Host,
		URI:           r.RequestURI,
		Proto:         r.Proto,
		RemoteAddr:    r.RemoteAddr,
		Headers:       reqHeaders,
		ContentType:   reqContentType,
		ContentLength: r.ContentLength,
	}

	// Wrap response writer to tee response body
	respBuf := boundedbuffer.New(h.MaxCaptureBytes)
	teeWriter := newTeeResponseWriter(w, respBuf)

	// Call next handler
	err := next.ServeHTTP(teeWriter, r)

	// Capture timing
	duration := time.Since(startTime)
	durationMs := float64(duration.Milliseconds())

	// Capture response headers (redacted)
	respHeaders := redactHeaders(teeWriter.Header(), h.RedactHeaders)
	respContentType := teeWriter.Header().Get("Content-Type")

	// Determine if response is SSE streaming
	isStreaming := isSSEContentType(respContentType)

	// Encode request body
	if captureReqBody {
		rawBody := reqBuf.Bytes()
		if decompressed, ok := decompressBody(rawBody, r.Header.Get("Content-Encoding")); ok {
			rawBody = decompressed
		}
		body, encoding := encodeBody(rawBody, reqContentType)
		reqRecord.Body = body
		reqRecord.BodyEncoding = encoding
		reqRecord.BodyTruncated = reqBuf.Truncated()
	}

	// Determine if we should include response body based on content_types filter
	captureRespBody := h.shouldCaptureBody(respContentType)

	// Build response record
	respRecord := &ResponseRecord{
		Status:      teeWriter.statusCode,
		Headers:     respHeaders,
		ContentType: respContentType,
	}
	if captureRespBody {
		rawBody := respBuf.Bytes()
		if decompressed, ok := decompressBody(rawBody, teeWriter.Header().Get("Content-Encoding")); ok {
			rawBody = decompressed
		}
		body, encoding := encodeBody(rawBody, respContentType)
		respRecord.Body = body
		respRecord.BodyEncoding = encoding
		respRecord.BodyTruncated = respBuf.Truncated()
	}

	if isStreaming {
		// Emit split records for SSE
		// Request record (emitted immediately)
		if err := h.sink.Write(&LogRecord{
			Timestamp:  startTime,
			RequestID:  requestID,
			RecordType: "request",
			Request:    reqRecord,
		}); err != nil {
			h.logger.Warn("failed to write request record", zap.String("request_id", requestID), zap.Error(err))
		}
		// Response record (emitted on stream close)
		if err := h.sink.Write(&LogRecord{
			Timestamp:  time.Now(),
			RequestID:  requestID,
			RecordType: "response",
			DurationMs: &durationMs,
			Response:   respRecord,
		}); err != nil {
			h.logger.Warn("failed to write response record", zap.String("request_id", requestID), zap.Error(err))
		}
	} else {
		// Emit single exchange record
		if err := h.sink.Write(&LogRecord{
			Timestamp:  startTime,
			RequestID:  requestID,
			RecordType: "exchange",
			DurationMs: &durationMs,
			Request:    reqRecord,
			Response:   respRecord,
		}); err != nil {
			h.logger.Warn("failed to write exchange record", zap.String("request_id", requestID), zap.Error(err))
		}
	}

	return err
}

// getRequestID extracts or generates a request ID.
func (h *Handler) getRequestID(r *http.Request) string {
	// Try Caddy's built-in request UUID via replacer
	if repl, ok := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer); ok {
		if id, _ := repl.GetString("http.request.uuid"); id != "" {
			return id
		}
	}

	// Try X-Request-Id header
	if id := r.Header.Get("X-Request-Id"); id != "" {
		return id
	}

	// Generate fallback UUID
	return uuid.New().String()
}

// shouldCaptureBody returns true if the content type matches the configured filter.
func (h *Handler) shouldCaptureBody(contentType string) bool {
	if len(h.ContentTypes) == 0 {
		return true // no filter = capture all
	}
	for _, ct := range h.ContentTypes {
		if strings.HasPrefix(contentType, ct) {
			return true
		}
	}
	return false
}

// isSSEContentType returns true if the content type indicates SSE streaming.
func isSSEContentType(contentType string) bool {
	ct := contentType
	if idx := strings.IndexByte(ct, ';'); idx != -1 {
		ct = strings.TrimSpace(ct[:idx])
	}
	return ct == "text/event-stream"
}

// Interface guards
var (
	_ caddy.Module                = (*Handler)(nil)
	_ caddy.Provisioner           = (*Handler)(nil)
	_ caddy.Validator             = (*Handler)(nil)
	_ caddy.CleanerUpper          = (*Handler)(nil)
	_ caddyhttp.MiddlewareHandler = (*Handler)(nil)
	_ caddyfile.Unmarshaler       = (*Handler)(nil)
)
