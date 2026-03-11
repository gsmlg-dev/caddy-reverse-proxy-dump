package caddyreverseproxydump

import (
	"encoding/json"
	"os"

	"go.uber.org/zap"
)

const defaultChannelSize = 4096

// JSONLSink writes LogRecords as JSONL to a file.
type JSONLSink struct {
	ch     chan *LogRecord
	done   chan struct{}
	logger *zap.Logger
}

// NewJSONLSink creates a new JSONL file sink with a background writer goroutine.
func NewJSONLSink(filePath string, logger *zap.Logger) (*JSONLSink, error) {
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	s := &JSONLSink{
		ch:     make(chan *LogRecord, defaultChannelSize),
		done:   make(chan struct{}),
		logger: logger,
	}

	go s.writeLoop(f)
	return s, nil
}

func (s *JSONLSink) writeLoop(f *os.File) {
	defer close(s.done)
	defer f.Close()

	enc := json.NewEncoder(f)
	for record := range s.ch {
		if err := enc.Encode(record); err != nil {
			s.logger.Warn("failed to write record", zap.Error(err))
		}
	}
}

// Write sends a record to the background writer. Non-blocking; drops on full channel.
func (s *JSONLSink) Write(record *LogRecord) error {
	select {
	case s.ch <- record:
	default:
		s.logger.Warn("sink channel full, dropping record",
			zap.String("request_id", record.RequestID))
	}
	return nil
}

// Close signals the writer goroutine to drain and stop.
func (s *JSONLSink) Close() error {
	close(s.ch)
	<-s.done
	return nil
}
