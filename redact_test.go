package caddyreverseproxydump

import (
	"net/http"
	"testing"
)

func TestRedactHeaders(t *testing.T) {
	h := http.Header{
		"Authorization": {"Bearer token123"},
		"Content-Type":  {"application/json"},
		"Cookie":        {"session=abc", "other=def"},
		"X-Api-Key":     {"sk-123"},
	}

	result := redactHeaders(h, defaultRedactHeaders)

	// Authorization should be redacted
	if result["Authorization"][0] != "[REDACTED]" {
		t.Errorf("Authorization not redacted: %v", result["Authorization"])
	}

	// Content-Type should NOT be redacted
	if result["Content-Type"][0] != "application/json" {
		t.Errorf("Content-Type was redacted: %v", result["Content-Type"])
	}

	// Cookie should be redacted (both values)
	for _, v := range result["Cookie"] {
		if v != "[REDACTED]" {
			t.Errorf("Cookie not redacted: %v", result["Cookie"])
		}
	}

	// X-Api-Key should be redacted
	if result["X-Api-Key"][0] != "[REDACTED]" {
		t.Errorf("X-Api-Key not redacted: %v", result["X-Api-Key"])
	}
}

func TestRedactHeadersCaseInsensitive(t *testing.T) {
	h := http.Header{}
	h.Set("authorization", "Bearer token") // lowercase

	result := redactHeaders(h, []string{"Authorization"})

	// http.Header canonicalizes keys, so "Authorization" is the key
	if result["Authorization"][0] != "[REDACTED]" {
		t.Errorf("authorization not redacted: %v", result)
	}
}

func TestRedactHeadersDoesNotMutateOriginal(t *testing.T) {
	h := http.Header{
		"Authorization": {"Bearer token123"},
	}

	redactHeaders(h, defaultRedactHeaders)

	// Original should be unchanged
	if h.Get("Authorization") != "Bearer token123" {
		t.Errorf("original header was mutated: %v", h)
	}
}

func TestRedactHeadersEmpty(t *testing.T) {
	h := http.Header{
		"Content-Type": {"text/plain"},
	}

	result := redactHeaders(h, []string{})

	if result["Content-Type"][0] != "text/plain" {
		t.Errorf("unexpected result: %v", result)
	}
}
