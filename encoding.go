package caddyreverseproxydump

import (
	"encoding/base64"
	"encoding/json"
	"mime"
	"strings"
)

// textApplicationTypes lists application/* MIME types treated as text.
var textApplicationTypes = map[string]bool{
	"application/json":                      true,
	"application/xml":                       true,
	"application/javascript":                true,
	"application/x-www-form-urlencoded":     true,
	"application/graphql":                   true,
}

// isTextContentType returns true if the content type should be encoded as text.
func isTextContentType(contentType string) bool {
	if contentType == "" {
		return true // default to text for empty content type
	}

	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "" {
		return true
	}

	// text/* is always text
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}

	// Check exact application types
	if textApplicationTypes[mediaType] {
		return true
	}

	// Check application/*+json and application/*+xml suffixes
	if strings.HasPrefix(mediaType, "application/") {
		if strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml") {
			return true
		}
	}

	return false
}

// isJSONContentType returns true if the content type is JSON.
func isJSONContentType(contentType string) bool {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "application/json" {
		return true
	}
	if strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json") {
		return true
	}
	return false
}

// encodeBody returns the encoded body and the encoding name.
// JSON bodies are returned as json.RawMessage so they embed as raw JSON objects.
func encodeBody(body []byte, contentType string) (any, string) {
	if isJSONContentType(contentType) && json.Valid(body) {
		return json.RawMessage(body), "json"
	}
	if isTextContentType(contentType) {
		return string(body), "text"
	}
	return base64.StdEncoding.EncodeToString(body), "base64"
}
