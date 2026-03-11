package caddyreverseproxydump

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestJSONLSinkWriteAndClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	sink, err := NewJSONLSink(path, zap.NewNop())
	if err != nil {
		t.Fatalf("failed to create sink: %v", err)
	}

	dur := float64(100)
	record := &LogRecord{
		Timestamp:  time.Now(),
		RequestID:  "test-123",
		RecordType: "exchange",
		DurationMs: &dur,
		Request: &RequestRecord{
			Method: "GET",
			URI:    "/test",
			Headers: map[string][]string{
				"Content-Type": {"text/plain"},
			},
			Body:         "hello",
			BodyEncoding: "text",
		},
		Response: &ResponseRecord{
			Status: 200,
			Headers: map[string][]string{
				"Content-Type": {"text/plain"},
			},
			Body:         "world",
			BodyEncoding: "text",
		},
	}

	if err := sink.Write(record); err != nil {
		t.Fatalf("write error: %v", err)
	}

	// Close drains the channel
	if err := sink.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}

	// Read back the file
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	var decoded LogRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.RequestID != "test-123" {
		t.Errorf("request_id: got %q, want %q", decoded.RequestID, "test-123")
	}
	if decoded.Request.Method != "GET" {
		t.Errorf("method: got %q, want %q", decoded.Request.Method, "GET")
	}
}

func TestJSONLSinkMultipleRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	sink, err := NewJSONLSink(path, zap.NewNop())
	if err != nil {
		t.Fatalf("failed to create sink: %v", err)
	}

	for i := 0; i < 10; i++ {
		sink.Write(&LogRecord{
			Timestamp:  time.Now(),
			RequestID:  "test",
			RecordType: "exchange",
		})
	}

	sink.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	// Count lines
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 10 {
		t.Errorf("expected 10 lines, got %d", lines)
	}
}

func TestJSONLSinkInvalidPath(t *testing.T) {
	_, err := NewJSONLSink("/nonexistent/dir/file.jsonl", zap.NewNop())
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestConsoleSinkWriteAndClose(t *testing.T) {
	sink := NewConsoleSink(zap.NewNop())

	record := &LogRecord{
		Timestamp:  time.Now(),
		RequestID:  "console-test",
		RecordType: "exchange",
	}

	// Should not error
	if err := sink.Write(record); err != nil {
		t.Fatalf("write error: %v", err)
	}

	if err := sink.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
}
