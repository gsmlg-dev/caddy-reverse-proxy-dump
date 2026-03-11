package caddyreverseproxydump

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
)

// teeResponseWriter wraps an http.ResponseWriter to tee response bytes
// into a bounded buffer while preserving optional interfaces.
type teeResponseWriter struct {
	http.ResponseWriter
	tee        io.Writer
	statusCode int
	wroteHeader bool
}

func newTeeResponseWriter(w http.ResponseWriter, tee io.Writer) *teeResponseWriter {
	return &teeResponseWriter{
		ResponseWriter: w,
		tee:            tee,
		statusCode:     http.StatusOK,
	}
}

func (w *teeResponseWriter) WriteHeader(statusCode int) {
	if !w.wroteHeader {
		w.statusCode = statusCode
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(statusCode)
	}
}

func (w *teeResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.statusCode = http.StatusOK
	}
	// Always write to the real writer first
	n, err := w.ResponseWriter.Write(b)
	// Tee to buffer — ignore tee errors (fail open)
	if n > 0 {
		w.tee.Write(b[:n])
	}
	return n, err
}

// Unwrap returns the underlying ResponseWriter for Caddy's interface checks.
func (w *teeResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Flush implements http.Flusher — required for SSE.
func (w *teeResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker — required for WebSocket upgrades.
func (w *teeResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

// ReadFrom implements io.ReaderFrom for efficient body forwarding.
func (w *teeResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.statusCode = http.StatusOK
	}
	// Wrap the reader to tee into our buffer
	teeReader := io.TeeReader(r, w.tee)
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(teeReader)
	}
	return io.Copy(w.ResponseWriter, teeReader)
}

// Verify interface compliance at compile time.
var (
	_ http.ResponseWriter = (*teeResponseWriter)(nil)
	_ http.Flusher        = (*teeResponseWriter)(nil)
	_ http.Hijacker       = (*teeResponseWriter)(nil)
	_ io.ReaderFrom       = (*teeResponseWriter)(nil)
)
