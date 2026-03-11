package caddyreverseproxydump

import (
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

func TestEncodeBody(t *testing.T) {
	// Text encoding
	body, enc := encodeBody([]byte("hello"), "application/json")
	if body != "hello" || enc != "text" {
		t.Errorf("expected text encoding, got body=%q enc=%q", body, enc)
	}

	// Base64 encoding
	body, enc = encodeBody([]byte{0x89, 0x50, 0x4E, 0x47}, "image/png")
	if enc != "base64" {
		t.Errorf("expected base64 encoding, got %q", enc)
	}
	if body != "iVBORw==" {
		t.Errorf("unexpected base64: %q", body)
	}
}
