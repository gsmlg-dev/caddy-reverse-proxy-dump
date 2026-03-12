package caddyreverseproxydump

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

// mockSink collects records for testing.
type mockSink struct {
	mu      sync.Mutex
	records []*LogRecord
}

func (s *mockSink) Write(record *LogRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return nil
}

func (s *mockSink) Close() error { return nil }

func (s *mockSink) getRecords() []*LogRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]*LogRecord, len(s.records))
	copy(cp, s.records)
	return cp
}

// echoHandler is a simple next handler that echoes the request body in the response.
type echoHandler struct{}

func (echoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	body, _ := io.ReadAll(r.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
	return nil
}

func TestHandlerServeHTTP(t *testing.T) {
	sink := &mockSink{}
	h := &Handler{
		MaxCaptureBytes: 1048576,
		RedactHeaders:   defaultRedactHeaders,
		sink:            sink,
		logger:          zap.NewNop(),
	}

	body := `{"model":"claude-opus-4-5"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")

	rec := httptest.NewRecorder()

	err := h.ServeHTTP(rec, req, echoHandler{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the response was passed through
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != body {
		t.Fatalf("expected body %q, got %q", body, rec.Body.String())
	}

	// Verify record was emitted
	records := sink.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.RecordType != "exchange" {
		t.Errorf("expected record_type=exchange, got %s", record.RecordType)
	}
	if record.Request == nil {
		t.Fatal("request record is nil")
	}
	if record.Response == nil {
		t.Fatal("response record is nil")
	}

	// Check request
	if record.Request.Method != "POST" {
		t.Errorf("expected POST, got %s", record.Request.Method)
	}
	reqBody, ok := record.Request.Body.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage for request body, got %T", record.Request.Body)
	}
	if string(reqBody) != body {
		t.Errorf("expected body %q, got %q", body, string(reqBody))
	}
	if record.Request.BodyEncoding != "json" {
		t.Errorf("expected json encoding, got %s", record.Request.BodyEncoding)
	}
	if record.Request.ContentType != "application/json" {
		t.Errorf("expected application/json, got %s", record.Request.ContentType)
	}

	// Check authorization was redacted
	if record.Request.Headers["Authorization"][0] != "[REDACTED]" {
		t.Errorf("Authorization not redacted: %v", record.Request.Headers["Authorization"])
	}

	// Check response
	if record.Response.Status != 200 {
		t.Errorf("expected status 200, got %d", record.Response.Status)
	}
	respBody, ok := record.Response.Body.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage for response body, got %T", record.Response.Body)
	}
	if string(respBody) != body {
		t.Errorf("expected response body %q, got %q", body, string(respBody))
	}

	// Check duration
	if record.DurationMs == nil || *record.DurationMs < 0 {
		t.Errorf("expected positive duration, got %v", record.DurationMs)
	}

	// Check request_id is present
	if record.RequestID == "" {
		t.Error("expected non-empty request_id")
	}
}

func TestHandlerSSEStreaming(t *testing.T) {
	sink := &mockSink{}
	h := &Handler{
		MaxCaptureBytes: 1048576,
		RedactHeaders:   defaultRedactHeaders,
		sink:            sink,
		logger:          zap.NewNop(),
	}

	// SSE handler
	sseHandler := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: hello\n\n"))
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"stream":true}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req, sseHandler)

	records := sink.getRecords()
	if len(records) != 2 {
		t.Fatalf("expected 2 split records, got %d", len(records))
	}

	// Both records should share the same request_id
	if records[0].RequestID != records[1].RequestID {
		t.Errorf("request IDs don't match: %s vs %s", records[0].RequestID, records[1].RequestID)
	}

	// First record should be request type
	if records[0].RecordType != "request" {
		t.Errorf("expected first record type=request, got %s", records[0].RecordType)
	}
	if records[0].Request == nil {
		t.Error("first record should have request data")
	}

	// Second record should be response type
	if records[1].RecordType != "response" {
		t.Errorf("expected second record type=response, got %s", records[1].RecordType)
	}
	if records[1].Response == nil {
		t.Error("second record should have response data")
	}
	if records[1].Response.ContentType != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", records[1].Response.ContentType)
	}
}

func TestHandlerRequestIDFromHeader(t *testing.T) {
	sink := &mockSink{}
	h := &Handler{
		MaxCaptureBytes: 1048576,
		RedactHeaders:   defaultRedactHeaders,
		sink:            sink,
		logger:          zap.NewNop(),
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-Id", "custom-id-123")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req, echoHandler{})

	records := sink.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].RequestID != "custom-id-123" {
		t.Errorf("expected custom-id-123, got %s", records[0].RequestID)
	}
}

func TestHandlerContentTypesFilter(t *testing.T) {
	sink := &mockSink{}
	h := &Handler{
		MaxCaptureBytes: 1048576,
		RedactHeaders:   defaultRedactHeaders,
		ContentTypes:    []string{"application/json"},
		sink:            sink,
		logger:          zap.NewNop(),
	}

	// Handler that returns text/plain (not in filter)
	textHandler := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("plain text response"))
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("request body"))
	req.Header.Set("Content-Type", "text/plain") // not in filter

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req, textHandler)

	records := sink.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	// Request body should be nil (content type not in filter)
	if records[0].Request.Body != nil {
		t.Errorf("expected nil request body, got %v", records[0].Request.Body)
	}

	// Response body should be nil (content type not in filter)
	if records[0].Response.Body != nil {
		t.Errorf("expected nil response body, got %v", records[0].Response.Body)
	}
}

func TestHandlerBodyTruncation(t *testing.T) {
	sink := &mockSink{}
	h := &Handler{
		MaxCaptureBytes: 10,
		RedactHeaders:   defaultRedactHeaders,
		sink:            sink,
		logger:          zap.NewNop(),
	}

	longBody := strings.Repeat("x", 100)
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(longBody))
	req.Header.Set("Content-Type", "text/plain")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req, echoHandler{})

	// Response should still have the full body
	if rec.Body.String() != longBody {
		t.Errorf("response body was altered")
	}

	records := sink.getRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	if !records[0].Request.BodyTruncated {
		t.Error("expected request body to be truncated")
	}
	truncBody, ok := records[0].Request.Body.(string)
	if !ok {
		t.Fatalf("expected string body for text/plain, got %T", records[0].Request.Body)
	}
	if len(truncBody) != 10 {
		t.Errorf("expected truncated body len=10, got %d", len(truncBody))
	}
}

func TestHandlerFailOpen(t *testing.T) {
	// Ensure handler doesn't break proxying even if something goes wrong
	sink := &mockSink{}
	h := &Handler{
		MaxCaptureBytes: 1048576,
		RedactHeaders:   defaultRedactHeaders,
		sink:            sink,
		logger:          zap.NewNop(),
	}

	// Handler that returns 500
	errorHandler := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	err := h.ServeHTTP(rec, req, errorHandler)
	if err != nil {
		t.Fatalf("handler should not return error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestLogRecordJSONSerialization(t *testing.T) {
	dur := float64(842)
	record := &LogRecord{
		Timestamp:  time.Date(2026, 3, 11, 10, 12, 33, 123000000, time.UTC),
		RequestID:  "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		RecordType: "exchange",
		DurationMs: &dur,
		Request: &RequestRecord{
			Method:     "POST",
			Scheme:     "https",
			Host:       "anthropic-api.gsmlg.dev",
			URI:        "/v1/messages",
			Proto:      "HTTP/2.0",
			RemoteAddr: "1.2.3.4:56789",
			Headers: map[string][]string{
				"Content-Type":  {"application/json"},
				"Authorization": {"[REDACTED]"},
			},
			Body:          json.RawMessage(`{"model":"claude-opus-4-5"}`),
			BodyEncoding:  "json",
			BodyTruncated: false,
			ContentType:   "application/json",
			ContentLength: 1234,
		},
		Response: &ResponseRecord{
			Status: 200,
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
			},
			Body:         json.RawMessage(`{"id":"msg_01..."}`),
			BodyEncoding: "json",
			ContentType:  "application/json",
		},
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(record); err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	// Verify it can be decoded back
	var decoded LogRecord
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if decoded.RequestID != record.RequestID {
		t.Errorf("request_id mismatch: %s vs %s", decoded.RequestID, record.RequestID)
	}
	if decoded.RecordType != "exchange" {
		t.Errorf("record_type mismatch: %s", decoded.RecordType)
	}
	if decoded.Request.Method != "POST" {
		t.Errorf("method mismatch: %s", decoded.Request.Method)
	}
}
