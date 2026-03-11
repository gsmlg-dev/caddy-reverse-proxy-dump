package caddyreverseproxydump

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTeeResponseWriterWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	var buf bytes.Buffer
	tw := newTeeResponseWriter(rec, &buf)

	tw.Write([]byte("hello"))
	tw.Write([]byte(" world"))

	if rec.Body.String() != "hello world" {
		t.Errorf("response body: got %q, want %q", rec.Body.String(), "hello world")
	}
	if buf.String() != "hello world" {
		t.Errorf("tee buffer: got %q, want %q", buf.String(), "hello world")
	}
}

func TestTeeResponseWriterWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	var buf bytes.Buffer
	tw := newTeeResponseWriter(rec, &buf)

	tw.WriteHeader(http.StatusCreated)
	if tw.statusCode != http.StatusCreated {
		t.Errorf("status: got %d, want %d", tw.statusCode, http.StatusCreated)
	}

	// Double WriteHeader should not change status
	tw.WriteHeader(http.StatusNotFound)
	if tw.statusCode != http.StatusCreated {
		t.Errorf("status after double write: got %d, want %d", tw.statusCode, http.StatusCreated)
	}
}

func TestTeeResponseWriterDefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	var buf bytes.Buffer
	tw := newTeeResponseWriter(rec, &buf)

	tw.Write([]byte("data"))
	if tw.statusCode != http.StatusOK {
		t.Errorf("default status: got %d, want %d", tw.statusCode, http.StatusOK)
	}
}

func TestTeeResponseWriterUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	var buf bytes.Buffer
	tw := newTeeResponseWriter(rec, &buf)

	if tw.Unwrap() != rec {
		t.Error("Unwrap should return the underlying ResponseWriter")
	}
}

func TestTeeResponseWriterFlush(t *testing.T) {
	rec := httptest.NewRecorder()
	var buf bytes.Buffer
	tw := newTeeResponseWriter(rec, &buf)

	// Should not panic even if underlying doesn't implement Flusher
	tw.Flush()

	// httptest.ResponseRecorder does implement Flusher
	if !rec.Flushed {
		t.Error("expected recorder to be flushed")
	}
}
