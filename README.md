# httprun

中文文档：[`README.zh-CN.md`](./README.zh-CN.md)

`httprun` is a Go CLI for running `.http` files and currently supports a subset of the JetBrains `.http` file format.

Current scope is intentionally narrow: it focuses on common HTTP request workflows and supports only part of the JetBrains `.http` format. It does not claim full compatibility with `ijhttp` or IDE scripting features.

## Status

Current implementation supports:

- Multiple `.http` files in one command
- File-level concurrency with `--jobs`
- Sequential execution inside each file
- Multiple requests per file via `###`
- Named requests via `# @name`
- Common HTTP methods such as `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`
- File variables via `@key = value`
- Variable interpolation in URL, headers, inline body, and external body files
- Public and private environment files
- Built-in variables `{{$uuid}}` and `{{$timestamp}}`
- Request options:
  - `# @timeout`
  - `# @connection-timeout`
  - `# @no-redirect`
  - `# @no-cookie-jar`
- External request body files via `< path/to/file`
- Cookie jar sharing within one file execution
- `run` and `validate` commands

Not supported yet:

- Pre-request scripts
- Response handler scripts
- Extracting variables from previous responses
- JavaScript APIs such as `client.*`
- WebSocket
- GraphQL-specific syntax
- gRPC
- OAuth and advanced auth helpers
- Multipart/form-data syntax helpers
- Directory scanning and recursive discovery

## Install

Install with `go install`:

```bash
go install github.com/smallfish/httprun/cmd/httprun@latest
```

Then verify:

```bash
httprun --help
```

## Commands

Top-level usage:

```text
Usage:
  httprun run [flags] <file.http> [more.http ...]
  httprun validate [flags] <file.http> [more.http ...]
```

### `run`

Execute one or more `.http` files.

```bash
httprun run examples/demo.http
httprun run --name ping examples/demo.http
```

Generic examples:

```bash
httprun run --jobs 4 a.http b.http c.http
httprun run --env dev --var base=https://example.com path/to/demo.http
```

Flags:

- `--name <request>`: execute only the named request
- `--env <env>`: load variables from `http-client.env.json` and `http-client.private.env.json`
- `--var key=value`: override variables, can be repeated
- `--jobs <n>`: process files concurrently, default `1`
- `--timeout <duration>`: default request timeout for `run`, default `30s`
- `--verbose`: print expanded request and response headers
- `--fail-http`: return non-zero if any response status is `>= 400`

### `validate`

Validate one or more `.http` files without sending requests.

```bash
httprun validate examples/demo.http
httprun validate --name ping examples/demo.http
```

Generic examples:

```bash
httprun validate --jobs 8 a.http b.http
httprun validate --name ping --env dev path/to/demo.http
```

Flags:

- `--name <request>`
- `--env <env>`
- `--var key=value`
- `--jobs <n>`

## Syntax Supported

### Multiple requests

Use `###` to split requests:

```http
###
GET https://example.com/health

###
POST https://example.com/items
Content-Type: application/json

{"name":"demo"}
```

### Named requests

```http
###
# @name login
POST https://example.com/login
Content-Type: application/json

{"user":"demo","pass":"secret"}
```

Run a single named request:

```bash
httprun run --name login demo.http
```

### File variables

```http
@base = https://example.com
@token = abc123

###
GET {{base}}/users
Authorization: Bearer {{token}}
```

### Environment files

`httprun` looks for environment files in the same directory as the `.http` file:

- `http-client.env.json`
- `http-client.private.env.json`

Example:

```json
{
  "dev": {
    "base": "https://dev.example.com",
    "token": "public-token"
  }
}
```

Use them with:

```bash
httprun run --env dev path/to/demo.http
```

Variable precedence is:

1. CLI `--var`
2. `http-client.env.json`
3. `http-client.private.env.json`
4. file variables such as `@base = ...`
5. built-in variables

### Built-in variables

Currently supported:

- `{{$uuid}}`
- `{{$timestamp}}`

Example:

```http
POST https://example.com/events
X-Request-Id: {{$uuid}}

{"createdAt":"{{$timestamp}}"}
```

### External body files

You can load the request body from a file:

```http
@payload = payload.json

###
POST https://example.com/items
Content-Type: application/json

< {{payload}}
```

The body file path is resolved relative to the `.http` file directory. Variable interpolation also applies inside the loaded file content.

### Request options

Request options are set with comment directives before the request line.

Timeout override:

```http
###
# @timeout 50 ms
GET {{base}}/slow
```

Connection timeout override:

```http
###
# @connection-timeout 2 s
GET {{base}}/health
```

Disable redirect following:

```http
###
# @no-redirect
GET {{base}}/redirect
```

Disable writing response cookies into the shared jar:

```http
###
# @no-cookie-jar
GET {{base}}/login
```

Inline form is also supported:

```http
###
# @no-redirect GET {{base}}/redirect
```

## Execution Model

Execution rules are:

- Requests inside one `.http` file are executed sequentially
- Files passed in one command can be executed concurrently with `--jobs`
- Output is printed in input file order even when files run concurrently
- Cookie jar is shared within one file execution
- Cookie jar is not shared across different files

This keeps per-file request order stable while still allowing parallel execution across independent test cases.

## Example

Example file: [examples/demo.http](./examples/demo.http)

```http
@base = https://httpbin.org

###
# @name ping
GET {{base}}/get
Accept: application/json

###
# @name createAnything
POST {{base}}/anything
Content-Type: application/json
X-Request-Id: {{$uuid}}

{
  "createdAt": "{{$timestamp}}",
  "source": "httprun"
}
```

Run it:

```bash
go run ./cmd/httprun run examples/demo.http
go run ./cmd/httprun run --name ping examples/demo.http
```

Additional example files in [`examples/`](./examples):

- [`examples/all_methods.http`](./examples/all_methods.http): common HTTP methods, variables, env files, external body files; runnable with `httprun run --env dev examples/all_methods.http`
- [`examples/request_options.http`](./examples/request_options.http): `@no-redirect` and `@no-cookie-jar`, using public `httpbin` endpoints
- [`examples/timeout.http`](./examples/timeout.http): request-level `@timeout`, using `httpbin /delay`
- [`examples/http-client.env.json`](./examples/http-client.env.json) and [`examples/http-client.private.env.json`](./examples/http-client.private.env.json): environment file examples

## Output and Exit Codes

- `run` returns `0` when all selected files complete successfully
- `run` returns `1` if any file fails
- `validate` returns `0` when all files validate successfully
- `validate` returns `1` if any file fails validation
- invalid CLI usage returns `2`

With `--fail-http`, HTTP responses with status `>= 400` are treated as command failures.

## Development

Build locally:

```bash
go build ./cmd/httprun
```

Or run directly:

```bash
go run ./cmd/httprun --help
```

Run tests:

```bash
go test ./...
```

Current tests cover:

- Parser behavior
- Variable resolution and precedence
- External body files
- Request directives
- Redirect and cookie behavior
- Timeout behavior
- `--name` selection
- Multi-file CLI execution
- File-level concurrency via `--jobs`
