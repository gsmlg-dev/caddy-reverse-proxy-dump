package boundedbuffer

import (
	"testing"
)

func TestBufferBasicWrite(t *testing.T) {
	buf := New(100)
	n, err := buf.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected n=5, got %d", n)
	}
	if string(buf.Bytes()) != "hello" {
		t.Fatalf("expected 'hello', got %q", buf.Bytes())
	}
	if buf.Truncated() {
		t.Fatal("should not be truncated")
	}
}

func TestBufferTruncation(t *testing.T) {
	buf := New(5)
	n, err := buf.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should report all bytes consumed (fail open)
	if n != 11 {
		t.Fatalf("expected n=11, got %d", n)
	}
	if string(buf.Bytes()) != "hello" {
		t.Fatalf("expected 'hello', got %q", buf.Bytes())
	}
	if !buf.Truncated() {
		t.Fatal("should be truncated")
	}
}

func TestBufferExactLimit(t *testing.T) {
	buf := New(5)
	buf.Write([]byte("hello"))
	if buf.Truncated() {
		t.Fatal("should not be truncated at exact limit")
	}
	if buf.Len() != 5 {
		t.Fatalf("expected len=5, got %d", buf.Len())
	}
}

func TestBufferMultipleWrites(t *testing.T) {
	buf := New(10)
	buf.Write([]byte("abc"))
	buf.Write([]byte("def"))
	buf.Write([]byte("ghijkl"))

	if string(buf.Bytes()) != "abcdefghij" {
		t.Fatalf("expected 'abcdefghij', got %q", buf.Bytes())
	}
	if !buf.Truncated() {
		t.Fatal("should be truncated")
	}
}

func TestBufferZeroLimit(t *testing.T) {
	buf := New(0)
	n, err := buf.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected n=5, got %d", n)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected len=0, got %d", buf.Len())
	}
	if !buf.Truncated() {
		t.Fatal("should be truncated")
	}
}

func TestBufferWriteAfterFull(t *testing.T) {
	buf := New(3)
	buf.Write([]byte("abc"))
	buf.Write([]byte("def"))

	if string(buf.Bytes()) != "abc" {
		t.Fatalf("expected 'abc', got %q", buf.Bytes())
	}
	if !buf.Truncated() {
		t.Fatal("should be truncated")
	}
}
