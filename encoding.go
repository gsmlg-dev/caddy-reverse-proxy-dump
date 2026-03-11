package caddyreverseproxydump

import (
	"encoding/base64"
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

// encodeBody returns the encoded body string and the encoding name.
func encodeBody(body []byte, contentType string) (string, string) {
	if isTextContentType(contentType) {
		return string(body), "text"
	}
	return base64.StdEncoding.EncodeToString(body), "base64"
}
