package caddyreverseproxydump

import "time"

// LogRecord is the top-level record written to the sink.
type LogRecord struct {
	Timestamp  time.Time       `json:"ts"`
	RequestID  string          `json:"request_id"`
	RecordType string          `json:"record_type"`            // "exchange", "request", or "response"
	DurationMs *float64        `json:"duration_ms,omitempty"`  // nil for split request records
	Request    *RequestRecord  `json:"request,omitempty"`
	Response   *ResponseRecord `json:"response,omitempty"`
}

// RequestRecord holds captured request data.
type RequestRecord struct {
	Method        string              `json:"method"`
	Scheme        string              `json:"scheme"`
	Host          string              `json:"host"`
	URI           string              `json:"uri"`
	Proto         string              `json:"proto"`
	RemoteAddr    string              `json:"remote_addr"`
	Headers       map[string][]string `json:"headers"`
	Body          string              `json:"body"`
	BodyEncoding  string              `json:"body_encoding"`
	BodyTruncated bool                `json:"body_truncated"`
	ContentType   string              `json:"content_type"`
	ContentLength int64               `json:"content_length"`
}

// ResponseRecord holds captured response data.
type ResponseRecord struct {
	Status        int                 `json:"status"`
	Headers       map[string][]string `json:"headers"`
	Body          string              `json:"body"`
	BodyEncoding  string              `json:"body_encoding"`
	BodyTruncated bool                `json:"body_truncated"`
	ContentType   string              `json:"content_type"`
}
