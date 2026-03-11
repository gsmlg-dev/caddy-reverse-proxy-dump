package caddyreverseproxydump

import (
	"net/http"
	"strings"
)

const redactedValue = "[REDACTED]"

// defaultRedactHeaders is the default set of headers to redact.
var defaultRedactHeaders = []string{
	"Authorization",
	"Cookie",
	"Set-Cookie",
	"Proxy-Authorization",
	"X-Api-Key",
}

// redactHeaders returns a copy of the headers with specified header values replaced.
func redactHeaders(h http.Header, redactList []string) map[string][]string {
	result := make(map[string][]string, len(h))
	for k, v := range h {
		result[k] = v
	}

	for _, name := range redactList {
		canonicalName := http.CanonicalHeaderKey(name)
		for k := range result {
			if strings.EqualFold(k, canonicalName) {
				redacted := make([]string, len(result[k]))
				for i := range redacted {
					redacted[i] = redactedValue
				}
				result[k] = redacted
			}
		}
	}

	return result
}
