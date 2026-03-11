# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Caddy HTTP handler middleware plugin (`http.handlers.reverse_proxy_dump`) that captures full HTTP request/response data (headers + bodies) for traffic passing through a reverse proxy. Output is JSONL to file or stdout. Filtering uses Caddy's native matcher syntax — no custom filter fields.

The PRD lives at `docs/PRD.md` and is the authoritative design reference.

## Build & Test

```bash
# Build custom Caddy binary with this plugin
xcaddy build --with github.com/gsmlg-dev/caddy-reverse-proxy-dump=.

# Run all tests
go test ./...

# Run a single test
go test -run TestHandlerServeHTTP ./...

# Run bounded buffer tests
go test ./internal/boundedbuffer/...

# Vet and lint
go vet ./...
```

## Architecture

The plugin sits before `reverse_proxy` in Caddy's handler chain and tees request/response bodies into bounded buffers without altering the proxied traffic.

**Key flow:** Client -> Caddy matcher -> `reverse_proxy_dump` handler -> tee request body -> wrap ResponseWriter -> next handler (`reverse_proxy`) -> tee response body -> async emit LogRecord to sink channel -> background writer goroutine writes JSONL.

### Core files (per PRD repo layout)

- `handler.go` — Module registration, `ServeHTTP`, implements `caddy.CleanerUpper`
- `caddyfile.go` — Caddyfile directive parser
- `response_writer.go` — Tee ResponseWriter preserving `http.Flusher`, `http.Hijacker`, `io.ReaderFrom`, `Unwrap()`
- `types.go` — `LogRecord`, `RequestRecord`, `ResponseRecord` structs
- `sink.go` — `Sink` interface (`Write`, `Close`)
- `sink_jsonl.go` / `sink_console.go` — Async sinks with single background writer goroutine per sink
- `redact.go` — Header redaction (`[REDACTED]` replacement)
- `encoding.go` — Body encoding decision (text vs base64) based on Content-Type
- `internal/boundedbuffer/` — Bounded `io.Writer` with truncation tracking

### Key design invariants

- **Fail open**: capture/write failures never block or break proxying
- **Single writer goroutine per sink**: serializes writes, no file locking needed
- **Bounded memory**: one bounded buffer (default 1 MiB) per active request
- **Backpressure**: channel full (4096 buffer) -> drop record + warn, never block request goroutine
- **Graceful shutdown**: `caddy.CleanerUpper` drains channel before closing file handle
- **No `http.Pusher`**: intentionally excluded (deprecated in browsers)

### Streaming (SSE)

For `text/event-stream` responses, two records are emitted sharing the same `request_id`:
1. `request` record — emitted when response begins
2. `response` record — emitted on stream close

### Body encoding

Text-safe types (`text/*`, `application/json`, `application/xml`, `application/*+json`, etc.) -> raw UTF-8 (`body_encoding: "text"`). Everything else -> base64.

### Request ID sourcing

`{http.request.uuid}` -> `X-Request-Id` header -> generated `uuid.New()` fallback.

## Caddy module interfaces to implement

- `caddy.Module` (module info)
- `caddy.Provisioner` (setup sink)
- `caddy.Validator` (validate config — file/console mutual exclusion)
- `caddy.CleanerUpper` (graceful shutdown)
- `caddyhttp.MiddlewareHandler` (`ServeHTTP`)
- `caddyfile.Unmarshaler` (Caddyfile parsing)

Register handler order: `order reverse_proxy_dump before reverse_proxy`

## Git Commits

- Omit "Generated with Claude Code" from commit messages
- Omit "Co-Authored-By: Claude" trailer
