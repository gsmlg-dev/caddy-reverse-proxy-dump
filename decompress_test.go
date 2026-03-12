package caddyreverseproxydump

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func gzipCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func deflateCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zstdCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecompressBody(t *testing.T) {
	original := []byte(`{"hello":"world"}`)

	tests := []struct {
		name            string
		body            []byte
		contentEncoding string
		wantDecompress  bool
		wantBody        []byte
	}{
		{
			name:            "gzip",
			body:            gzipCompress(t, original),
			contentEncoding: "gzip",
			wantDecompress:  true,
			wantBody:        original,
		},
		{
			name:            "x-gzip",
			body:            gzipCompress(t, original),
			contentEncoding: "x-gzip",
			wantDecompress:  true,
			wantBody:        original,
		},
		{
			name:            "deflate",
			body:            deflateCompress(t, original),
			contentEncoding: "deflate",
			wantDecompress:  true,
			wantBody:        original,
		},
		{
			name:            "zstd",
			body:            zstdCompress(t, original),
			contentEncoding: "zstd",
			wantDecompress:  true,
			wantBody:        original,
		},
		{
			name:            "no encoding",
			body:            original,
			contentEncoding: "",
			wantDecompress:  false,
			wantBody:        original,
		},
		{
			name:            "unsupported br",
			body:            []byte("compressed-br-data"),
			contentEncoding: "br",
			wantDecompress:  false,
			wantBody:        []byte("compressed-br-data"),
		},
		{
			name:            "empty body",
			body:            nil,
			contentEncoding: "gzip",
			wantDecompress:  false,
			wantBody:        nil,
		},
		{
			name:            "invalid gzip data",
			body:            []byte("not-gzip"),
			contentEncoding: "gzip",
			wantDecompress:  false,
			wantBody:        []byte("not-gzip"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := decompressBody(tt.body, tt.contentEncoding)
			if ok != tt.wantDecompress {
				t.Errorf("decompressBody() ok = %v, want %v", ok, tt.wantDecompress)
			}
			if !bytes.Equal(got, tt.wantBody) {
				t.Errorf("decompressBody() body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}
