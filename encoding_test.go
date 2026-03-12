package caddyreverseproxydump

import (
	"encoding/json"
	"testing"
)

func TestIsTextContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"text/plain", true},
		{"text/html", true},
		{"text/event-stream", true},
		{"text/plain; charset=utf-8", true},
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"application/xml", true},
		{"application/javascript", true},
		{"application/x-www-form-urlencoded", true},
		{"application/graphql", true},
		{"application/vnd.api+json", true},
		{"application/soap+xml", true},
		{"application/octet-stream", false},
		{"image/png", false},
		{"audio/mp3", false},
		{"application/pdf", false},
		{"", true}, // default to text
	}

	for _, tt := range tests {
		t.Run(tt.ct, func(t *testing.T) {
			got := isTextContentType(tt.ct)
			if got != tt.want {
				t.Errorf("isTextContentType(%q) = %v, want %v", tt.ct, got, tt.want)
			}
		})
	}
}

func TestIsJSONContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"application/vnd.api+json", true},
		{"text/plain", false},
		{"application/xml", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ct, func(t *testing.T) {
			got := isJSONContentType(tt.ct)
			if got != tt.want {
				t.Errorf("isJSONContentType(%q) = %v, want %v", tt.ct, got, tt.want)
			}
		})
	}
}

func TestEncodeBody(t *testing.T) {
	// JSON encoding — valid JSON body with JSON content type
	body, enc := encodeBody([]byte(`{"hello":"world"}`), "application/json")
	if enc != "json" {
		t.Errorf("expected json encoding, got %q", enc)
	}
	if _, ok := body.(json.RawMessage); !ok {
		t.Errorf("expected json.RawMessage, got %T", body)
	}

	// JSON content type but invalid JSON — falls back to text
	body, enc = encodeBody([]byte("not-json"), "application/json")
	if enc != "text" {
		t.Errorf("expected text encoding for invalid JSON, got %q", enc)
	}
	if s, ok := body.(string); !ok || s != "not-json" {
		t.Errorf("expected string body, got %T %v", body, body)
	}

	// Text encoding — non-JSON text type
	body, enc = encodeBody([]byte("hello"), "text/plain")
	if enc != "text" {
		t.Errorf("expected text encoding, got %q", enc)
	}
	if s, ok := body.(string); !ok || s != "hello" {
		t.Errorf("expected string body, got %T %v", body, body)
	}

	// Base64 encoding
	body, enc = encodeBody([]byte{0x89, 0x50, 0x4E, 0x47}, "image/png")
	if enc != "base64" {
		t.Errorf("expected base64 encoding, got %q", enc)
	}
	if s, ok := body.(string); !ok || s != "iVBORw==" {
		t.Errorf("unexpected base64: %v", body)
	}
}
