package caddyreverseproxydump

import (
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
)

func TestUnmarshalCaddyfile(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Handler
		wantErr bool
	}{
		{
			name: "file sink with all options",
			input: `reverse_proxy_dump {
				file /var/log/dump.jsonl
				max_capture_bytes 2048
				redact_headers Authorization X-Api-Key
				content_types application/json text/event-stream
			}`,
			want: Handler{
				SinkType:        "file",
				FilePath:        "/var/log/dump.jsonl",
				MaxCaptureBytes: 2048,
				RedactHeaders:   []string{"Authorization", "X-Api-Key"},
				ContentTypes:    []string{"application/json", "text/event-stream"},
			},
		},
		{
			name: "console sink",
			input: `reverse_proxy_dump {
				console
			}`,
			want: Handler{
				SinkType: "console",
			},
		},
		{
			name: "file only",
			input: `reverse_proxy_dump {
				file /tmp/test.jsonl
			}`,
			want: Handler{
				SinkType: "file",
				FilePath: "/tmp/test.jsonl",
			},
		},
		{
			name: "unknown subdirective",
			input: `reverse_proxy_dump {
				unknown_option foo
			}`,
			wantErr: true,
		},
		{
			name: "file without path",
			input: `reverse_proxy_dump {
				file
			}`,
			wantErr: true,
		},
		{
			name: "max_capture_bytes invalid",
			input: `reverse_proxy_dump {
				max_capture_bytes notanumber
			}`,
			wantErr: true,
		},
		{
			name: "redact_headers without args",
			input: `reverse_proxy_dump {
				redact_headers
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := caddyfile.NewTestDispenser(tt.input)
			var h Handler
			err := h.UnmarshalCaddyfile(d)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if h.SinkType != tt.want.SinkType {
				t.Errorf("SinkType: got %q, want %q", h.SinkType, tt.want.SinkType)
			}
			if h.FilePath != tt.want.FilePath {
				t.Errorf("FilePath: got %q, want %q", h.FilePath, tt.want.FilePath)
			}
			if h.MaxCaptureBytes != tt.want.MaxCaptureBytes {
				t.Errorf("MaxCaptureBytes: got %d, want %d", h.MaxCaptureBytes, tt.want.MaxCaptureBytes)
			}
			if len(h.RedactHeaders) != len(tt.want.RedactHeaders) {
				t.Errorf("RedactHeaders: got %v, want %v", h.RedactHeaders, tt.want.RedactHeaders)
			}
			if len(h.ContentTypes) != len(tt.want.ContentTypes) {
				t.Errorf("ContentTypes: got %v, want %v", h.ContentTypes, tt.want.ContentTypes)
			}
		})
	}
}
