package caddyreverseproxydump

// Sink is the interface for writing log records.
type Sink interface {
	Write(record *LogRecord) error
	Close() error
}
