package caddyreverseproxydump

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// decompressBody attempts to decompress the body based on Content-Encoding.
// Returns the decompressed bytes and true if decompression succeeded,
// or the original bytes and false if no decompression was needed or it failed.
func decompressBody(body []byte, contentEncoding string) ([]byte, bool) {
	if len(body) == 0 || contentEncoding == "" {
		return body, false
	}

	enc := strings.TrimSpace(strings.ToLower(contentEncoding))

	var reader io.ReadCloser
	var err error

	switch enc {
	case "gzip", "x-gzip":
		reader, err = gzip.NewReader(bytes.NewReader(body))
	case "deflate":
		reader = flate.NewReader(bytes.NewReader(body))
	case "zstd":
		var dec *zstd.Decoder
		dec, err = zstd.NewReader(bytes.NewReader(body))
		if err != nil {
			return body, false
		}
		defer dec.Close()
		decompressed, err := io.ReadAll(dec)
		if err != nil {
			return body, false
		}
		return decompressed, true
	case "br":
		// brotli not supported — return as-is
		return body, false
	default:
		return body, false
	}

	if err != nil {
		return body, false
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return body, false
	}
	return decompressed, true
}
