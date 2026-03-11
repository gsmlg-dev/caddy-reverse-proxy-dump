package caddyreverseproxydump

import (
	"strconv"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// parseCaddyfile sets up the handler from Caddyfile tokens.
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var handler Handler
	err := handler.UnmarshalCaddyfile(h.Dispenser)
	if err != nil {
		return nil, err
	}
	return &handler, nil
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (h *Handler) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	d.Next() // consume directive name

	hasSink := false
	for d.NextBlock(0) {
		switch d.Val() {
		case "file":
			if hasSink {
				return d.Errf("file and console are mutually exclusive")
			}
			if !d.NextArg() {
				return d.ArgErr()
			}
			h.SinkType = "file"
			h.FilePath = d.Val()
			hasSink = true

		case "console":
			if hasSink {
				return d.Errf("file and console are mutually exclusive")
			}
			h.SinkType = "console"
			hasSink = true

		case "max_capture_bytes":
			if !d.NextArg() {
				return d.ArgErr()
			}
			val, err := strconv.Atoi(d.Val())
			if err != nil {
				return d.Errf("invalid max_capture_bytes: %v", err)
			}
			h.MaxCaptureBytes = val

		case "redact_headers":
			args := d.RemainingArgs()
			if len(args) == 0 {
				return d.ArgErr()
			}
			h.RedactHeaders = args

		case "content_types":
			args := d.RemainingArgs()
			if len(args) == 0 {
				return d.ArgErr()
			}
			h.ContentTypes = args

		default:
			return d.Errf("unrecognized subdirective: %s", d.Val())
		}
	}

	return nil
}
