# Aither

[![Go Reference](https://pkg.go.dev/badge/github.com/coderianx/Aither.svg)](https://pkg.go.dev/github.com/coderianx/Aither)

A local-first chat client for OpenAI-compatible LLM providers. Aither runs a single
Go binary that serves both an HTTP API and an embedded web UI, and keeps every
conversation in a SQLite database on your own machine.

Point it at an [OpenRouter](https://openrouter.ai/) or [Groq](https://groq.com/) key —
or any custom OpenAI-compatible endpoint — and start chatting.

```
┌─────────────┐        ┌──────────────────────────────┐
│  Web UI     │  HTTP  │  Aither server (Go)          │
│  (embedded) │ ─────▶ │  chi router + SSE streaming  │
└─────────────┘        └───────────┬──────────────────┘
                                   │
                        ┌──────────┴──────────┐
                        ▼                     ▼
                 ┌─────────────┐      ┌──────────────────┐
                 │  SQLite     │      │  LLM provider    │
                 │  ~/.aither  │      │  /chat/completions│
                 └─────────────┘      └──────────────────┘
```

## Features

- **Streaming replies** — tokens are delivered over server-sent events and rendered
  as they arrive, with a stop button that keeps whatever was already generated.
- **Full history replay** — every request sends the conversation's stored history
  back to the provider, so context survives restarts without any client bookkeeping.
- **Multiple providers** — connect as many as you like and switch models per
  conversation from a searchable picker.
- **Markdown rendering** — headings, lists, tables of code, quotes and links, with a
  dependency-free syntax highlighter covering 17 languages.
- **Zero-config storage** — the schema is created on first run; the database lives
  outside the repository by default.
- **Single binary** — the entire UI is compiled in via `//go:embed`. No Node, no
  bundler, no CDN requests.

## Requirements

- **Go 1.27 or newer**
- **A C compiler** (gcc or clang) — the SQLite driver uses cgo, so `CGO_ENABLED=1`
  is required. This is the Go default; if you build with cgo disabled, the build
  fails with a message about the missing driver.

## Installation

### With `go install`

The fastest route — one command, no clone, no build step:

```bash
go install github.com/coderianx/Aither@latest
```

Go fetches the source, compiles it and drops the binary in `$(go env GOPATH)/bin`
(`~/go/bin` on a default setup). The binary is named after the repository, so it is
invoked with a capital A:

```bash
Aither
```

If your shell cannot find it, put that directory on your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Because the SQLite driver uses cgo, `go install` needs a working C compiler and
`CGO_ENABLED=1` (the Go default). With cgo disabled the command fails with a message
about the missing driver.

### From source

```bash
git clone https://github.com/coderianx/Aither.git
cd Aither
go build .        # produces ./Aither, named after the module
./Aither
```

Use `go build -o aither .` if you prefer a lower-case name; either way the result is
a single self-contained binary.

## Usage

Open <http://localhost:8000>. On first launch Aither asks for a provider: pick
OpenRouter or Groq, paste an API key, and you are ready to chat. Choose a model from
the picker in the top bar and send a message.

## Configuration

| Flag    | Default                   | Description                          |
| ------- | ------------------------- | ------------------------------------ |
| `-port` | `:8000`                   | Address and port to listen on        |
| `-db`   | `~/.aither/aither.db`     | Path to the SQLite database file     |

The default directory is created automatically. To keep everything inside the
project folder instead, run `Aither -db ./aither.db` (the path is covered by
`.gitignore`).

## Providers

Aither speaks the OpenAI chat-completions format, which covers a large ecosystem.

| Provider     | Base URL                        | Notes                                  |
| ------------ | ------------------------------- | -------------------------------------- |
| `openrouter` | `https://openrouter.ai/api/v1`  | Default when no base URL is given      |
| `groq`       | `https://api.groq.com/openai/v1`| Default when no base URL is given      |
| `custom`     | *you supply it*                 | Any OpenAI-compatible endpoint         |

A provider must expose `POST {base_url}/chat/completions` and, for the model picker,
`GET {base_url}/models`. Model lists are fetched once per session and cached in the
browser.

> **API keys are stored in plain text** in the local SQLite database. Aither never
> sends a key anywhere except the provider it belongs to, and the server does not
> echo keys back in responses — but anyone with read access to `~/.aither/aither.db`
> can read them.

## API

All endpoints are JSON and live under `/api/v1`. Errors use the shape
`{"error": "message"}` with a matching HTTP status.

### Providers

| Method   | Path                        | Description                        |
| -------- | --------------------------- | ---------------------------------- |
| `GET`    | `/providers`                | List configured providers          |
| `POST`   | `/providers`                | Add a provider                     |
| `DELETE` | `/providers/{id}`           | Remove a provider                  |
| `GET`    | `/providers/{id}/models`    | Proxy the provider's model catalogue |

<details>
<summary>Example</summary>

```bash
curl -X POST http://localhost:8000/api/v1/providers \
  -H 'Content-Type: application/json' \
  -d '{"provider_name":"groq","api_key":"gsk_...","base_url":""}'
```

```json
{ "id": 1, "message": "Provider created successfully" }
```

</details>

### Conversations

| Method   | Path                        | Description                              |
| -------- | --------------------------- | ---------------------------------------- |
| `GET`    | `/conversations`            | List conversations, most recent first    |
| `POST`   | `/conversations`            | Create a conversation                    |
| `GET`    | `/conversations/{id}`       | Fetch one conversation                   |
| `PATCH`  | `/conversations/{id}`       | Update title, model, provider or prompt  |
| `DELETE` | `/conversations/{id}`       | Delete a conversation and all its messages |

`POST` requires a `model`. `provider_id` is optional at creation time but must be set
before sending a message. On `PATCH`, only the fields you include are changed.

```bash
curl -X POST http://localhost:8000/api/v1/conversations \
  -H 'Content-Type: application/json' \
  -d '{"provider_id":1,"model":"llama-3.3-70b-versatile","system_prompt":"Be concise."}'
```

### Messages

| Method   | Path                             | Description                     |
| -------- | -------------------------------- | ------------------------------- |
| `GET`    | `/conversations/{id}/messages`   | Full message history, oldest first |
| `POST`   | `/conversations/{id}/messages`   | Send a message, get a reply     |

The `POST` body is `{"content": "...", "stream": false}`.

**Non-streaming** (`stream: false`) returns both stored records:

```json
{
  "user_message":      { "id": 1, "role": "user",      "content": "..." },
  "assistant_message": { "id": 2, "role": "assistant", "content": "..." }
}
```

**Streaming** (`stream: true`) responds with `text/event-stream`. Each event carries a
JSON payload:

| Event          | Payload                                   |
| -------------- | ----------------------------------------- |
| `user_message` | The stored user message record            |
| `delta`        | `{"content": "next chunk"}`               |
| `done`         | The stored assistant message record       |
| `error`        | `{"error": "message"}`                    |

Because the response is already committed by the time a failure can occur, streaming
errors arrive in-band as an `error` event rather than as an HTTP status.

```bash
curl -N -X POST http://localhost:8000/api/v1/conversations/1/messages \
  -H 'Content-Type: application/json' \
  -d '{"content":"Explain goroutines briefly","stream":true}'
```

## How a message is handled

1. The body is validated and the conversation is loaded with its provider.
2. The user message is written to the database **first**, so it is part of the
   history sent on this very request.
3. The stored history is replayed in order — system prompt first, then up to the
   last 200 messages — and posted to the provider.
4. The reply is streamed to the client. The assembled text is written to the
   database once the stream ends, along with a bump to `updated_at`.
5. If the client disconnects mid-stream, the partial reply is still stored, so what
   you saw on screen matches what is in the history.

Provider failures are mapped to friendly messages (`Provider rejected the API key`,
`Provider rate limit reached`, and so on). The raw upstream body is never forwarded,
so it cannot leak into logs or UI.

## Project structure

```
main.go                      flags, DB path resolution, routes, static handler
internal/
  handlers/                  HTTP layer: provider, conversation, message, chat, response helpers
  models/                    request and response types
  provider/                  OpenAI-compatible client (chat, streaming, model list)
  store/                     SQLite access: connection, schema, per-entity queries
web/
  index.html                 markup and the inline icon sprite
  style.css                  design tokens plus components
  app.js                     API client, markdown renderer, syntax highlighter
  embed.go                   //go:embed declarations
```

The web assets are embedded, so **a CSS or JS change requires a rebuild** for the
server to serve it. During editing, `go build . && ./Aither` is the loop.

## Database

Three tables are created automatically on first connect:

- `providers` — name, API key and base URL.
- `conversations` — title, model, system prompt and the provider it belongs to. The
  provider reference is `ON DELETE SET NULL`, so deleting a provider leaves its
  conversations intact.
- `messages` — role and content, `ON DELETE CASCADE` from its conversation.

The connection runs in WAL mode with a 5-second busy timeout and the pool capped at
one connection, which is the safest configuration for SQLite's single-writer model.

To start over, stop the server and delete the database file:

```bash
rm ~/.aither/aither.db*
```

## Development

```bash
go build .              # build the binary (./Aither)
go vet ./...            # static checks
gofmt -l .              # list files needing formatting
```

There is no test suite yet.

## Repository notes

`.gitattributes` marks `web/**/*.{html,css,js}` as `linguist-detectable=false`, so the
hand-written and embedded client stays out of GitHub's language breakdown and the
stats bar reflects the Go code. HTML, CSS and JS are still stored and served
normally — only the language statistics ignore them.
