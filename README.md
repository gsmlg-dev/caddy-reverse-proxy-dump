# caddy-reverse-proxy-dump

Caddy plugin that captures full HTTP request/response data (headers + bodies) for traffic passing through a reverse proxy. Output is JSONL to file or stdout. Intended for debugging and traffic inspection — **not for production use**.

## Install

Build a custom Caddy binary with [xcaddy](https://github.com/caddyserver/xcaddy):

```bash
xcaddy build --with github.com/gsmlg-dev/caddy-reverse-proxy-dump
```

## Usage

```caddy
example.com {
    reverse_proxy_dump {
        file /var/log/caddy/dump.jsonl
        max_capture_bytes 1048576
        redact_headers Authorization Cookie Set-Cookie X-Api-Key
        content_types application/json text/event-stream
    }
    reverse_proxy https://api.example.com
}
```

### Subdirectives

| Directive | Description | Default |
|---|---|---|
| `file <path>` | Write JSONL to file | — |
| `console` | Write JSONL to stdout | *(default)* |
| `max_capture_bytes <n>` | Max bytes to capture per body | `1048576` (1 MiB) |
| `redact_headers <h>...` | Headers to replace with `[REDACTED]` | `Authorization Cookie Set-Cookie` |
| `content_types <t>...` | Only capture bodies matching these types | *(all)* |

`file` and `console` are mutually exclusive. Filtering which requests to capture is done with Caddy's native [request matchers](https://caddyserver.com/docs/caddyfile/matchers).

### Output format

Each line is a JSON object (`LogRecord`):

```json
{
  "ts": "2026-03-11T12:00:00Z",
  "request_id": "abc-123",
  "record_type": "exchange",
  "duration_ms": 42.0,
  "request": {
    "method": "POST",
    "scheme": "https",
    "host": "example.com",
    "uri": "/api/v1/messages",
    "proto": "HTTP/2.0",
    "remote_addr": "1.2.3.4:5678",
    "headers": { "Content-Type": ["application/json"] },
    "body": "{\"model\":\"claude-sonnet-4-20250514\"}",
    "body_encoding": "text",
    "body_truncated": false,
    "content_type": "application/json",
    "content_length": 32
  },
  "response": {
    "status": 200,
    "headers": { "Content-Type": ["application/json"] },
    "body": "{\"id\":\"msg_01\"}",
    "body_encoding": "text",
    "body_truncated": false,
    "content_type": "application/json"
  }
}
```

For SSE (`text/event-stream`) responses, two records are emitted sharing the same `request_id`: a `"request"` record when the response begins and a `"response"` record on stream close.

Binary bodies are base64-encoded (`body_encoding: "base64"`).

## Design

- **Fail open** — capture failures never block or break proxying
- **Bounded memory** — one bounded buffer (default 1 MiB) per active request/response
- **Async writes** — records are sent to a buffered channel; a single background goroutine writes to the sink
- **Backpressure** — if the channel is full (4096 records), the record is dropped with a warning

## License

See [LICENSE](LICENSE).
