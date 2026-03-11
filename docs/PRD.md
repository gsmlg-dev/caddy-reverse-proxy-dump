# PRD: Caddy Reverse Proxy Dump

**Repository:** `github.com/gsmlg-dev/caddy-reverse-proxy-dump`
**Project Name:** `caddy-reverse-proxy-dump`
**Caddy Module ID:** `http.handlers.reverse_proxy_dump`

---

## 1. Overview

`caddy-reverse-proxy-dump` is a Caddy plugin that registers as an HTTP handler middleware (`http.handlers.reverse_proxy_dump`). It captures and persists full HTTP request and response data — headers and bodies — for traffic passing through it.

Filtering is handled entirely by Caddy's native matcher syntax. The plugin adds no custom filter fields; operators use standard `[<matcher>]` expressions to scope capture.

Primary use case:

```caddy
anthropic-api.gsmlg.dev {
  reverse_proxy_dump {
    file /var/log/caddy/reverse_proxy_dump.jsonl
    max_capture_bytes 1048576
    redact_headers Authorization Cookie Set-Cookie X-Api-Key
    content_types application/json text/event-stream
  }
  reverse_proxy https://api.anthropic.com
}
```

---

## 2. Problem Statement

Caddy's built-in logging covers access metadata but not full payload inspection. Developers proxying API traffic (e.g. via a custom `ANTHROPIC_BASE_URL`) need a passive, reliable way to capture complete HTTP exchanges — headers and bodies, including streaming responses — without replacing Caddy's upstream transport or writing a separate middleware chain.

---

## 3. Goals

* Capture full request and response headers and bodies as an HTTP handler middleware.
* Delegate request filtering entirely to Caddy's native matcher system.
* Support two output sinks: file (JSONL) and console (stdout).
* Replace sensitive header values with `[REDACTED]`, consistent with Caddy's log module.
* Preserve streaming behavior (SSE, chunked) via tee mode.
* Fail open — proxying continues if capture or writing fails.
* Graceful shutdown — drain pending records on Caddy reload/stop.
* File rotation deferred to a future version.

---

## 4. Non-Goals

* Custom filter fields (`only_hosts`, `only_paths`, `methods`) — use Caddy matchers instead.
* Traffic mutation or request rewriting.
* Packet-level or TLS-level sniffing.
* Distributed log shipping in v1.
* Replay or search features in v1.
* File rotation in v1.
* UI in v1.

---

## 5. Architecture

### 5.1 Integration point

The plugin registers as an HTTP handler middleware via `http.handlers.reverse_proxy_dump`. It sits in the handler chain before `reverse_proxy` and wraps the request/response lifecycle to tee bodies into bounded buffers.

```text
client
  └─> [Caddy matcher] ──── no match ──> pass through
                      └─── matched ──> reverse_proxy_dump handler
                                         ├─ capture request (headers + body tee)
                                         ├─ wrap response writer
                                         └─> next handler (reverse_proxy)
                                              ├─ forward to upstream
                                              └─ response flows back through tee writer
                                                   └─> async Sink.Write(record)
```

### 5.2 Request path

1. Caddy matcher determines whether this handler processes the request.
2. Copy and redact request headers.
3. Tee request body into bounded buffer via `io.TeeReader`; the original body stream is preserved for downstream handlers.
4. If `content_types` filter is set, check the request's `Content-Type` to decide whether to capture the request body.
5. Pass the request to the next handler (typically `reverse_proxy`).

### 5.3 Response path

1. Wrap `http.ResponseWriter` with tee writer before calling next handler.
2. Response bytes flow through the tee writer — forwarded to client immediately, copied into bounded buffer.
3. On response completion, check the response's `Content-Type` against `content_types` filter to decide whether to include the captured response body in the record.
4. Capture response status and headers (post-redaction).
5. Stop buffering at `max_capture_bytes`; set `body_truncated: true`.
6. Emit `LogRecord` to async sink channel.

**`content_types` filter timing:** The filter is checked against the request `Content-Type` before body tee setup (to skip request body capture early), and against the response `Content-Type` after the response completes (to decide whether to include the already-teed response body in the emitted record). The response body is always teed through the bounded buffer regardless of content-type — the filter only controls whether the captured bytes appear in the emitted record. This avoids the need to predict the response content-type before the upstream responds.

### 5.4 Response writer interface preservation

The wrapped writer must preserve optional interfaces via runtime assertion, following Caddy's `caddyhttp.responseWriterWrapper` pattern:

* `http.Flusher` — required for SSE
* `http.Hijacker` — required for WebSocket upgrades
* `io.ReaderFrom` — for efficient body forwarding
* `Unwrap() http.ResponseWriter` — for Caddy's own interface checks

`http.Pusher` is intentionally excluded — HTTP/2 server push is deprecated in all major browsers.

### 5.5 Graceful shutdown

The handler implements `caddy.CleanerUpper`. On Caddy reload or stop:

1. The sink's `Close()` method is called.
2. The internal channel is closed, signaling the background writer goroutine.
3. The goroutine drains all remaining records from the channel before returning.
4. The file handle (if applicable) is closed after drain completes.

This ensures no records are lost during config reloads.

---

## 6. Caddyfile Syntax

```caddy
reverse_proxy_dump {
    file /var/log/caddy/reverse_proxy_dump.jsonl
    # or: console
    max_capture_bytes 1048576
    redact_headers Authorization Cookie Set-Cookie X-Api-Key
    content_types application/json text/event-stream
}
```

All configuration is nested inside the `reverse_proxy_dump` block. This keeps the plugin's namespace self-contained and avoids polluting other directives.

### Sub-directives

| Directive | Type | Default | Description |
|---|---|---|---|
| `file <path>` | string | — | Write JSONL to file. Mutually exclusive with `console`. |
| `console` | flag | — | Write JSONL to stdout. Mutually exclusive with `file`. |
| `max_capture_bytes` | int | `1048576` | Max bytes to buffer per body. Excess is truncated. |
| `redact_headers` | string list | see §7.4 | Header names to redact. Case-insensitive. Value replaced with `[REDACTED]`. |
| `content_types` | string list | (all) | Only include bodies matching these content-type prefixes in emitted records. |

`file` and `console` are mutually exclusive. Specifying both is a config error. Specifying neither defaults to `console`.

### Matcher scoping

All request filtering is done via Caddy's native matcher syntax:

```caddy
# Capture only /v1/messages
@anthropic path /v1/messages*
reverse_proxy_dump @anthropic {
    file /var/log/caddy/dump.jsonl
}
reverse_proxy https://api.anthropic.com

# Capture all traffic through this site
reverse_proxy_dump {
    console
}
reverse_proxy https://api.anthropic.com
```

### Handler ordering

`reverse_proxy_dump` must be ordered **before** `reverse_proxy` in the handler chain. Add an `order` directive in the global options block:

```caddy
{
    order reverse_proxy_dump before reverse_proxy
}
```

---

## 7. Configuration Specification

### 7.1 Go struct

```go
type Handler struct {
    SinkType        string   `json:"sink_type,omitempty"`         // "file" or "console"
    FilePath        string   `json:"file_path,omitempty"`         // output path for file sink
    MaxCaptureBytes int      `json:"max_capture_bytes,omitempty"`
    RedactHeaders   []string `json:"redact_headers,omitempty"`
    ContentTypes    []string `json:"content_types,omitempty"`
}
```

### 7.2 Defaults

* `sink_type`: `"console"`
* `max_capture_bytes`: `1048576` (1 MiB)
* `redact_headers`: `["Authorization", "Cookie", "Set-Cookie", "Proxy-Authorization", "X-Api-Key"]`
* `content_types`: empty (capture all)

### 7.3 Body encoding policy

| Content-Type | Encoding | `body_encoding` value |
|---|---|---|
| `text/*` | raw UTF-8 string | `"text"` |
| `application/json` | raw UTF-8 string | `"text"` |
| `application/xml` | raw UTF-8 string | `"text"` |
| `application/javascript` | raw UTF-8 string | `"text"` |
| `application/x-www-form-urlencoded` | raw UTF-8 string | `"text"` |
| `application/graphql` | raw UTF-8 string | `"text"` |
| `application/*+json`, `application/*+xml` | raw UTF-8 string | `"text"` |
| all others | base64 | `"base64"` |

Content-Type parameters (e.g. `charset=utf-8`) are stripped before matching. The full list of text-safe application types is maintained in `encoding.go`.

### 7.4 Default redacted headers

`Authorization`, `Cookie`, `Set-Cookie`, `Proxy-Authorization`, `X-Api-Key`

Matching is case-insensitive. Value replaced with the literal string `[REDACTED]`.

---

## 8. Sink Interface

```go
type Sink interface {
    Write(record *LogRecord) error
    Close() error
}
```

### Implementations (v1)

| File | Sink |
|---|---|
| `sink.go` | `Sink` interface definition |
| `sink_jsonl.go` | JSONL file sink |
| `sink_console.go` | stdout sink |

Both use a buffered channel and a single background writer goroutine. The single-writer goroutine serializes all writes, so no additional file locking is needed — this is an explicit invariant.

**Channel buffer size:** `4096` records (constant `defaultChannelSize`). This balances burst tolerance against memory. Not configurable in v1; may be exposed in v2 if operators report issues.

**Backpressure:** When the channel is full, the record is dropped and a warning is emitted via Caddy's logger. The request goroutine must never block on sink writes.

**File rotation:** Not supported in v1. Operators should use external tools (`logrotate`, `systemd`). Planned for v2.

---

## 9. Data Model

### 9.1 Request ID

The `request_id` field is sourced in this order:

1. `{http.request.uuid}` — Caddy's built-in per-request UUID (available via replacer).
2. `X-Request-Id` request header — if the Caddy variable is unavailable.
3. Generated `uuid.New()` — fallback.

This ensures every record has a unique, stable identifier that can be correlated with Caddy's access logs.

### 9.2 Record types

For normal (non-streaming) exchanges, a single `exchange` record is emitted containing both request and response.

For long-lived streams (`text/event-stream`, chunked with no `Content-Length`), two records sharing the same `request_id` are emitted:

* **request record** — emitted when the response begins; `response` field is absent.
* **response record** — emitted on stream close; `request` field is absent.

Consumers must correlate split records by `request_id`.

**Streaming detection:** A response is classified as streaming if its `Content-Type` is `text/event-stream` (after stripping parameters). This check happens after the upstream responds.

### 9.3 Denormalized fields

`content_type` is present on both `RequestRecord` and `ResponseRecord` as a convenience field denormalized from the headers. It duplicates `headers["Content-Type"]` to simplify consumer filtering without header traversal.

### 9.4 Normal exchange record

```json
{
  "ts": "2026-03-11T10:12:33.123Z",
  "request_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "record_type": "exchange",
  "duration_ms": 842,
  "request": {
    "method": "POST",
    "scheme": "https",
    "host": "anthropic-api.gsmlg.dev",
    "uri": "/v1/messages",
    "proto": "HTTP/2.0",
    "remote_addr": "1.2.3.4:56789",
    "headers": {
      "Content-Type": ["application/json"],
      "Authorization": ["[REDACTED]"]
    },
    "body": "{\"model\":\"claude-opus-4-5\",...}",
    "body_encoding": "text",
    "body_truncated": false,
    "content_type": "application/json",
    "content_length": 1234
  },
  "response": {
    "status": 200,
    "headers": {
      "Content-Type": ["application/json"]
    },
    "body": "{\"id\":\"msg_01...\"}",
    "body_encoding": "text",
    "body_truncated": false,
    "content_type": "application/json"
  }
}
```

### 9.5 Split stream records

**Request record** (emitted when response begins):

```json
{
  "ts": "2026-03-11T10:12:33.123Z",
  "request_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "record_type": "request",
  "request": {
    "method": "POST",
    "uri": "/v1/messages",
    "headers": { "Authorization": ["[REDACTED]"] },
    "body": "{...}",
    "body_encoding": "text",
    "body_truncated": false,
    "content_type": "application/json",
    "content_length": 1234
  }
}
```

**Response record** (emitted on stream close):

```json
{
  "ts": "2026-03-11T10:12:45.001Z",
  "request_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "record_type": "response",
  "duration_ms": 11878,
  "response": {
    "status": 200,
    "headers": { "Content-Type": ["text/event-stream"] },
    "body": "ZGF0YTogey4uLn0K...",
    "body_encoding": "base64",
    "body_truncated": true,
    "content_type": "text/event-stream"
  }
}
```

---

## 10. Non-Functional Requirements

| ID | Requirement |
|---|---|
| NFR-1 | Must not alter reverse proxy retry, transport, or upstream behavior |
| NFR-2 | Bounded memory — no unbounded buffering; one bounded buffer per active request |
| NFR-3 | Must not break SSE or chunked response flush semantics |
| NFR-4 | Internal errors logged via Caddy's logger (`zap.Logger`) |
| NFR-5 | Sensitive headers replaced with `[REDACTED]` before any persistence |
| NFR-6 | Buildable with `xcaddy` |
| NFR-7 | Single background writer goroutine per sink — serializes all file writes without locking |
| NFR-8 | Graceful drain on Caddy reload/stop via `caddy.CleanerUpper` |

---

## 11. Failure Behavior

| Scenario | Behavior |
|---|---|
| Body capture failure | Log warning, continue proxying. Record emitted without body. |
| Sink write failure | Log warning, continue proxying. Record lost. |
| Sink channel full | Drop record, log warning. Request goroutine never blocks. |
| Output file unavailable at startup | Fail Caddy config validation with clear error message. |
| Output file becomes unavailable at runtime | Sink write fails, logged as warning. Proxying continues. |

---

## 12. Repo Layout

```text
caddy-reverse-proxy-dump/
  go.mod
  go.sum
  README.md
  LICENSE
  PRD.md
  handler.go            # Caddy module registration, HTTP middleware, ServeHTTP
  caddyfile.go          # Caddyfile directive parser
  response_writer.go    # Tee response writer, interface preservation
  redact.go             # Header [REDACTED] replacement logic
  encoding.go           # Body encoding (text vs base64), content-type matching
  sink.go               # Sink interface
  sink_jsonl.go         # JSONL file sink (async background writer)
  sink_console.go       # stdout sink (async background writer)
  types.go              # LogRecord, RequestRecord, ResponseRecord
  handler_test.go       # Handler unit tests
  redact_test.go        # Redaction unit tests
  encoding_test.go      # Encoding + content-type matching tests
  internal/
    boundedbuffer/
      buffer.go         # Bounded io.Writer with truncation tracking
      buffer_test.go    # Buffer unit tests
  docs/
    parsing.md          # Parse guide (schema, decoding, examples)
  test/
    integration/        # End-to-end Caddy tests
```

---

## 13. Milestones

### Milestone 1: MVP

* HTTP handler middleware registration (`http.handlers.reverse_proxy_dump`)
* Caddyfile parsing for all sub-directives
* `file` JSONL sink with async background writer
* Request/response tee capture via bounded buffer
* Header redaction with default list
* `max_capture_bytes` truncation with `body_truncated` flag
* Body encoding (text vs base64) based on content-type
* Request ID sourcing (`{http.request.uuid}` → `X-Request-Id` → generated UUID)
* Graceful shutdown via `caddy.CleanerUpper`
* Unit tests for handler, redaction, encoding, bounded buffer

### Milestone 2: Console sink + streaming

* `console` stdout sink
* Interface-preserving response writer (`Flusher`, `Hijacker`, `ReaderFrom`, `Unwrap`)
* SSE integration tests
* `content_types` filter (request-side early check + response-side post-check)
* Split records — for `text/event-stream` responses, emit `request` record when response begins; emit `response` record on stream close; both share the same `request_id`
* Integration tests with SSE upstream

### Milestone 2.1 (v1.1): JSON body field redaction

* `redact_body_fields` config — list of JSON Pointer paths per RFC 6901 (e.g. `/api_key`)
* Wildcard extension for array traversal: `/messages/~/content` where `~` matches all array indices (explicit extension over RFC 6901; documented as non-standard)
* Applies only to `body_encoding: "text"` records with `application/json` content type
* Redacted field values replaced with `"[REDACTED]"`
* Parse guide updated with redaction examples

### Milestone 3: Production hardening

* Async sink backpressure metrics (dropped record count exposed via Caddy's metrics)
* Benchmark and memory profiling
* Deployment docs
* **Parse guide** — `docs/parsing.md` documenting the JSONL schema, body encoding policy, redaction tokens, and working examples in `jq`, Python, and Go

### Milestone 4: File rotation + extended sinks

* File rotation support (size-based and time-based)
* Configurable channel buffer size
* Pluggable sink registration
* Webhook / object storage sinks

---

## 14. Acceptance Criteria

### MVP

* `xcaddy build --with github.com/gsmlg-dev/caddy-reverse-proxy-dump` succeeds
* Caddyfile parses `reverse_proxy_dump { file ... }` without error
* Handler ordering directive (`order reverse_proxy_dump before reverse_proxy`) works
* Request body captured and forwarded to downstream handler unmodified
* Response body captured and returned to client unmodified
* JSONL record includes headers (`[REDACTED]` where configured) and bodies (correct `body_encoding`)
* Truncation sets `body_truncated: true` at configured limit
* `request_id` is populated via Caddy UUID, header, or generated fallback
* Caddy reload drains pending records without loss
* Unit tests pass for handler, redaction, encoding, and bounded buffer

### Streaming

* SSE responses reach client incrementally with no added latency
* `http.Flusher` preserved through response wrapper (verified by integration test)
* Tee writer does not block on full buffer
* Split `request` and `response` records emitted with matching `request_id`

---

## 15. Risks

| Risk | Mitigation |
|---|---|
| SSE flush semantics broken by response wrapper | Preserve `http.Flusher` via `Unwrap()` + runtime assertion following Caddy's `responseWriterWrapper` pattern; integration test with SSE upstream |
| Sensitive data persisted to disk | Default redaction list; security notice in README; `redact_body_fields` in v1.1 |
| Large payloads cause memory pressure | Bounded buffer with 1 MiB default; one buffer per active request |
| Disk growth from JSONL output | External rotation recommendation; rotation in v2 |
| Channel backpressure under burst | 4096-record buffer; drop-and-warn on full; metrics in v1.1 |
| File write contention from concurrent requests | Single background writer goroutine per sink — no contention by design |
| Records lost on Caddy reload | `caddy.CleanerUpper` drains channel before closing file handle |

---

## 16. Resolved Design Decisions

| Question | Decision | Rationale |
|---|---|---|
| Integration point | HTTP handler middleware, not `reverse_proxy` sub-directive | Cleaner Caddy module registration; avoids hooking into `reverse_proxy`'s internal parser; composable with any downstream handler |
| Caddyfile namespace | All config nested under `reverse_proxy_dump {}` | Avoids polluting `reverse_proxy`'s sub-directive space |
| JSON body field-level redaction | Deliver in v1.1 alongside header redaction infrastructure | Keeps MVP scope tight |
| JSON body field syntax | JSON Pointer (RFC 6901) with `~` wildcard for array indices | Standard-adjacent; JSONPath is too complex for this use case |
| Sampling rate | Out of scope — not planned | |
| Split request/response records | Yes — for `text/event-stream` responses | Emit request record when response begins; response record on stream close |
| Admin API for recent captures | Out of scope — not planned | |
| `http.Pusher` support | Excluded | HTTP/2 server push is deprecated in all major browsers |
| `content_types` filter timing | Request CT checked before tee setup; response CT checked after completion | Tee always runs; filter only controls whether body appears in emitted record |
| Request ID source | `{http.request.uuid}` → `X-Request-Id` header → `uuid.New()` | Correlates with Caddy access logs |
| Channel buffer size | 4096 records, not configurable in v1 | Balances burst tolerance vs memory; expose in v2 if needed |
| Concurrency model | Single writer goroutine per sink | Eliminates file locking; all serialization via channel |

---

## 17. Parse Guide (`docs/parsing.md`)

To be completed upon Milestone 2 delivery. Must cover:

* **File format** — JSONL, one record per line, no wrapping array
* **Schema reference** — all fields in `LogRecord`, `RequestRecord`, `ResponseRecord` with types and nullable notes
* **Record type semantics** — when `exchange` vs split `request`/`response` records are emitted; correlation by `request_id`
* **Denormalized fields** — `content_type` duplicates header data for consumer convenience
* **Body decoding** — how to branch on `body_encoding: "text" | "base64"` to recover the original payload
* **Text content-type mapping** — full list of content types treated as text vs base64, including `application/*+json` and `application/*+xml` subtypes
* **Redacted headers** — `[REDACTED]` is a literal string value, not a structured token; how to detect and skip
* **Truncation** — how to interpret `body_truncated: true`; partial bodies are valid UTF-8/base64 up to `max_capture_bytes`; truncated base64 decodes to a partial payload that may not be valid JSON/SSE
* **Streaming records** — what a truncated SSE body looks like; partial `data:` chunks are expected
* **Working examples** in:
  * `jq` — filter by record type, decode base64 body, extract fields
  * Python — iterate records, decode bodies, redaction-aware field access
  * Go — unmarshal into `LogRecord`, base64 decode helper

The guide must be kept in sync with schema changes in `types.go`.

---

## 18. Security Notice

This plugin captures request and response payloads including prompt content, API keys (if not redacted), and potentially sensitive user data. Deploy only in controlled, trusted environments. Review `redact_headers` configuration before production use. Consider `redact_body_fields` (v1.1) for JSON payloads containing embedded credentials or PII.