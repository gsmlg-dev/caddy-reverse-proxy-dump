package caddyreverseproxydump

import (
	"encoding/json"
	"os"

	"go.uber.org/zap"
)

// ConsoleSink writes LogRecords as JSONL to stdout.
type ConsoleSink struct {
	ch     chan *LogRecord
	done   chan struct{}
	logger *zap.Logger
}

// NewConsoleSink creates a new console sink with a background writer goroutine.
func NewConsoleSink(logger *zap.Logger) *ConsoleSink {
	s := &ConsoleSink{
		ch:     make(chan *LogRecord, defaultChannelSize),
		done:   make(chan struct{}),
		logger: logger,
	}

	go s.writeLoop()
	return s
}

func (s *ConsoleSink) writeLoop() {
	defer close(s.done)

	enc := json.NewEncoder(os.Stdout)
	for record := range s.ch {
		if err := enc.Encode(record); err != nil {
			s.logger.Warn("failed to write record to console", zap.Error(err))
		}
	}
}

// Write sends a record to the background writer. Non-blocking; drops on full channel.
func (s *ConsoleSink) Write(record *LogRecord) error {
	select {
	case s.ch <- record:
	default:
		s.logger.Warn("sink channel full, dropping record",
			zap.String("request_id", record.RequestID))
	}
	return nil
}

// Close signals the writer goroutine to drain and stop.
func (s *ConsoleSink) Close() error {
	close(s.ch)
	<-s.done
	return nil
}
