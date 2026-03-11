# Parse Guide

This document describes the JSONL output format of `caddy-reverse-proxy-dump` and how to work with captured records.

## File Format

Output is [JSONL](https://jsonlines.org/) — one JSON object per line, no wrapping array. Each line is a self-contained record.

```
{"ts":"2026-03-11T10:12:33.123Z","request_id":"a1b2...","record_type":"exchange",...}
{"ts":"2026-03-11T10:12:34.456Z","request_id":"b2c3...","record_type":"exchange",...}
```

## Schema Reference

### LogRecord (top-level)

| Field | Type | Description |
|---|---|---|
| `ts` | string (RFC 3339) | Timestamp when the request was received |
| `request_id` | string | Unique identifier for the exchange |
| `record_type` | string | `"exchange"`, `"request"`, or `"response"` |
| `duration_ms` | number \| null | Request duration in milliseconds. Absent for split `request` records |
| `request` | RequestRecord \| null | Request data. Absent for split `response` records |
| `response` | ResponseRecord \| null | Response data. Absent for split `request` records |

### RequestRecord

| Field | Type | Description |
|---|---|---|
| `method` | string | HTTP method (GET, POST, etc.) |
| `scheme` | string | `"http"` or `"https"` |
| `host` | string | Request host |
| `uri` | string | Request URI including query string |
| `proto` | string | Protocol version (e.g. `"HTTP/2.0"`) |
| `remote_addr` | string | Client IP and port |
| `headers` | object | Map of header name to string array |
| `body` | string | Request body (text or base64 encoded) |
| `body_encoding` | string | `"text"` or `"base64"` |
| `body_truncated` | boolean | `true` if body exceeded `max_capture_bytes` |
| `content_type` | string | Convenience field, duplicated from headers |
| `content_length` | number | Content-Length header value (-1 if unknown) |

### ResponseRecord

| Field | Type | Description |
|---|---|---|
| `status` | number | HTTP status code |
| `headers` | object | Map of header name to string array |
| `body` | string | Response body (text or base64 encoded) |
| `body_encoding` | string | `"text"` or `"base64"` |
| `body_truncated` | boolean | `true` if body exceeded `max_capture_bytes` |
| `content_type` | string | Convenience field, duplicated from headers |

## Record Types

### `exchange`

Normal (non-streaming) HTTP exchanges produce a single `exchange` record containing both `request` and `response` fields.

### Split records: `request` + `response`

For SSE (`text/event-stream`) responses, two records are emitted sharing the same `request_id`:

1. **`request`** — emitted when the response begins. Contains `request` field only; `response` is absent.
2. **`response`** — emitted when the stream closes. Contains `response` field only; `request` is absent.

Correlate split records by `request_id`.

## Body Encoding

The `body_encoding` field indicates how the body is encoded:

- **`"text"`** — raw UTF-8 string. Use directly.
- **`"base64"`** — standard base64 encoding. Decode before use.

### Text content types

These content types produce `body_encoding: "text"`:

- `text/*` (all text types)
- `application/json`
- `application/xml`
- `application/javascript`
- `application/x-www-form-urlencoded`
- `application/graphql`
- `application/*+json` (e.g. `application/vnd.api+json`)
- `application/*+xml` (e.g. `application/soap+xml`)

All other types produce `body_encoding: "base64"`.

Content-Type parameters (e.g. `charset=utf-8`) are stripped before matching.

## Redacted Headers

Header values matching the `redact_headers` configuration are replaced with the literal string `[REDACTED]`. Default redacted headers: `Authorization`, `Cookie`, `Set-Cookie`, `Proxy-Authorization`, `X-Api-Key`.

```json
"headers": {
  "Authorization": ["[REDACTED]"],
  "Content-Type": ["application/json"]
}
```

To detect redacted values, check for exact string equality with `"[REDACTED]"`.

## Truncation

When `body_truncated` is `true`, the body was cut at `max_capture_bytes` (default 1 MiB). The captured portion is valid UTF-8 (for text) or valid base64 (for binary), but the decoded content may be incomplete (e.g. partial JSON, partial SSE events).

## Working Examples

### jq

```bash
# List all exchange records
cat dump.jsonl | jq 'select(.record_type == "exchange")'

# Extract request bodies from POST requests
cat dump.jsonl | jq 'select(.request.method == "POST") | .request.body' -r

# Decode base64 response body
cat dump.jsonl | jq -r 'select(.response.body_encoding == "base64") | .response.body' | base64 -d

# Filter by content type
cat dump.jsonl | jq 'select(.request.content_type == "application/json")'

# Correlate split SSE records
cat dump.jsonl | jq -s 'group_by(.request_id) | .[] | select(length > 1)'
```

### Python

```python
import json
import base64

with open("dump.jsonl") as f:
    for line in f:
        record = json.loads(line)

        # Skip redacted headers
        if req := record.get("request"):
            for name, values in req["headers"].items():
                if values == ["[REDACTED]"]:
                    continue
                print(f"{name}: {values}")

        # Decode body
        if resp := record.get("response"):
            body = resp["body"]
            if resp["body_encoding"] == "base64":
                body = base64.b64decode(body)
            else:
                body = body.encode("utf-8")
            print(f"Status: {resp['status']}, Body length: {len(body)}")
```

### Go

```go
package main

import (
    "bufio"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "os"
)

type LogRecord struct {
    Timestamp  string          `json:"ts"`
    RequestID  string          `json:"request_id"`
    RecordType string          `json:"record_type"`
    DurationMs *float64        `json:"duration_ms,omitempty"`
    Request    *RequestRecord  `json:"request,omitempty"`
    Response   *ResponseRecord `json:"response,omitempty"`
}

type RequestRecord struct {
    Method        string              `json:"method"`
    URI           string              `json:"uri"`
    Headers       map[string][]string `json:"headers"`
    Body          string              `json:"body"`
    BodyEncoding  string              `json:"body_encoding"`
    BodyTruncated bool                `json:"body_truncated"`
    ContentType   string              `json:"content_type"`
}

type ResponseRecord struct {
    Status        int                 `json:"status"`
    Headers       map[string][]string `json:"headers"`
    Body          string              `json:"body"`
    BodyEncoding  string              `json:"body_encoding"`
    BodyTruncated bool                `json:"body_truncated"`
    ContentType   string              `json:"content_type"`
}

func decodeBody(body, encoding string) ([]byte, error) {
    if encoding == "base64" {
        return base64.StdEncoding.DecodeString(body)
    }
    return []byte(body), nil
}

func main() {
    f, _ := os.Open("dump.jsonl")
    defer f.Close()

    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        var rec LogRecord
        json.Unmarshal(scanner.Bytes(), &rec)
        fmt.Printf("[%s] %s %s\n", rec.RecordType, rec.RequestID, rec.Timestamp)
    }
}
```
