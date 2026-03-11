package boundedbuffer

import "sync"

// Buffer is a bounded io.Writer that tracks truncation.
// Once the limit is reached, additional writes are silently discarded
// and Truncated() returns true.
type Buffer struct {
	mu        sync.Mutex
	buf       []byte
	limit     int
	truncated bool
}

// New creates a Buffer with the given byte limit.
func New(limit int) *Buffer {
	return &Buffer{
		buf:   make([]byte, 0, min(limit, 64*1024)),
		limit: limit,
	}
}

// Write implements io.Writer. Bytes beyond the limit are discarded.
func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	remaining := b.limit - len(b.buf)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil // fail open: report all bytes consumed
	}
	if len(p) > remaining {
		b.buf = append(b.buf, p[:remaining]...)
		b.truncated = true
	} else {
		b.buf = append(b.buf, p...)
	}
	return len(p), nil
}

// Bytes returns the buffered data.
func (b *Buffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf
}

// Truncated returns true if any bytes were discarded due to the limit.
func (b *Buffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

// Len returns the current buffer length.
func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.buf)
}
