# httprun

中文文档：[`README.zh-CN.md`](./README.zh-CN.md)

`httprun` is a Go CLI for running `.http` files. It intentionally supports a focused subset of the JetBrains `.http` format for common HTTP request workflows. It does not aim to be fully compatible with `ijhttp` or IDE scripting features.

## At A Glance

| Area | Support |
| --- | --- |
| Commands | `run`, `validate` |
| File execution | Multiple `.http` files per command, file-level concurrency via `--jobs` |
| Request layout | `###` request separators, `# @name` named requests |
| Methods | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD` |
| Variables | File variables, env files, CLI `--var`, built-ins `{{$uuid}}` / `{{$timestamp}}` |
| Request content | URL, headers, inline body, external body file via `< path` |
| Request options | `@timeout`, `@connection-timeout`, `@no-redirect`, `@no-cookie-jar` |
| Assertions | `# @assert <expression>` with `status`, `body`, `json.<path>`, `header.<name>` |
| Output model | Compact summary by default, `--verbose` for expanded request/response details |
| Execution model | Sequential inside one file, concurrent across files, cookie jar shared within a file |

## Quick Start

Install:

```bash
go install github.com/smallfish/httprun/cmd/httprun@latest
httprun --help
```

Minimal file:

```http
@base = https://httpbin.org

###
# @name ping
# @assert status == 200
GET {{base}}/get
Accept: application/json
```

Run it:

```bash
httprun run demo.http
httprun run --name ping demo.http
httprun validate demo.http
```

## Common Commands

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
httprun run --jobs 4 a.http b.http c.http
httprun run --env dev --var base=https://example.com path/to/demo.http
```

| Flag | Meaning |
| --- | --- |
| `--name <request>` | Execute only the named request |
| `--env <env>` | Load variables from `http-client.env.json` and `http-client.private.env.json` |
| `--var key=value` | Override variables, repeatable |
| `--jobs <n>` | Number of files to process concurrently, default `1` |
| `--timeout <duration>` | Default request timeout, default `30s` |
| `--verbose` | Print expanded request and response details |
| `--fail-http` | Return non-zero on HTTP status `>= 400` |

### `validate`

Validate one or more `.http` files without sending requests.

```bash
httprun validate examples/demo.http
httprun validate --name ping examples/demo.http
httprun validate --jobs 8 a.http b.http
httprun validate --name ping --env dev path/to/demo.http
```

Supported flags: `--name`, `--env`, `--var`, `--jobs`.

## Most Common Patterns

### Multiple requests

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

Run one named request:

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

Notes:

- File variables use `@key = value`.
- Request metadata such as `# @name`, `# @assert`, and `# @timeout` is recognized only in comment directives, not in variable declarations.
- `@name = foo` is still a normal variable declaration, but reusing directive-like names is not recommended because it makes files harder to read.

### Environment files

`httprun` looks in the same directory as the `.http` file for:

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

Variable precedence:

1. CLI `--var`
2. `http-client.env.json`
3. `http-client.private.env.json`
4. File variables such as `@base = ...`
5. Built-in variables

### Built-in variables

Supported built-ins:

- `{{$uuid}}`
- `{{$timestamp}}`

```http
POST https://example.com/events
X-Request-Id: {{$uuid}}

{"createdAt":"{{$timestamp}}"}
```

### External body files

```http
@payload = payload.json

###
POST https://example.com/items
Content-Type: application/json

< {{payload}}
```

The body file path is resolved relative to the `.http` file directory. Variable interpolation also applies inside the loaded file content.

### Placement rules

- File variables, request names, request directives, and assertions all belong before the request line.
- Headers belong after the request line and before the first blank line.
- The request body starts after the first blank line.
- Anything written after the body is treated as body content, not as request metadata.

## Request Directives

Directives are comment lines before the request line.

```http
###
# @timeout 50s
# @connection-timeout 2s
# @no-redirect
GET {{base}}/slow
```

Inline form is supported for request-line directives:

```http
###
# @no-redirect GET {{base}}/redirect
```

| Directive | Meaning |
| --- | --- |
| `# @timeout 50s` | Override request timeout |
| `# @connection-timeout 2s` | Override connection timeout |
| `# @no-redirect` | Do not follow redirects |
| `# @no-cookie-jar` | Do not write response cookies into the shared jar |

## Assertions

Assertions are comment directives before the request line.

```http
###
# @assert status == 200
# @assert body contains hello
# @assert json.data.user.name == "demo"
# @assert header.Content-Type contains "application/json"
GET {{base}}/profile
```

### Supported Subjects And Operators

| Subject | Operators | Example |
| --- | --- | --- |
| `status` | `==`, `!=`, `>`, `>=`, `<`, `<=` | `# @assert status == 200` |
| `body` | `==`, `!=`, `contains`, `not_contains`, `exists`, `not_exists` | `# @assert body contains hello` |
| `json.<path>` | `==`, `!=`, `>`, `>=`, `<`, `<=`, `exists`, `not_exists` | `# @assert json.data.count >= 2` |
| `header.<name>` | `==`, `!=`, `contains`, `not_contains`, `exists`, `not_exists` | `# @assert header.X-Trace-Id exists` |

### Assertion Notes

- `@assert` must appear before the request line. If you write it after the body, it is treated as body content rather than an assertion.
- `json.<path>` uses dot-path syntax. Arrays use numeric segments such as `json.data.items.0.id`.
- JSON comparison values must be valid JSON. Strings must be quoted, booleans use `true` / `false`, and numbers use JSON number syntax.
- If any assertion fails, execution of the current file stops immediately. Remaining requests in that file are skipped.

## Execution Model

- Requests inside one `.http` file run sequentially.
- Files in one command can run concurrently with `--jobs`.
- Output is printed in input file order even when files run concurrently.
- Cookie jar is shared within one file execution.
- Cookie jar is not shared across different files.

## Examples

Primary example:

- [`examples/demo.http`](./examples/demo.http): minimal end-to-end example

Additional examples:

- [`examples/all_methods.http`](./examples/all_methods.http): common HTTP methods, variables, env files, external body files
- [`examples/assertions.http`](./examples/assertions.http): successful assertions across `status`, `body`, `json.*`, `header.*`, and multiple operators
- [`examples/assertions_failure.http`](./examples/assertions_failure.http): intentional assertion failure, non-zero exit, and skipped follow-up requests
- [`examples/request_options.http`](./examples/request_options.http): `@no-redirect` and `@no-cookie-jar`
- [`examples/timeout.http`](./examples/timeout.http): request-level `@timeout`
- [`examples/http-client.env.json`](./examples/http-client.env.json) and [`examples/http-client.private.env.json`](./examples/http-client.private.env.json): environment file examples

Try them:

```bash
go run ./cmd/httprun run examples/demo.http
go run ./cmd/httprun run examples/assertions.http
go run ./cmd/httprun run examples/assertions_failure.http
```

## Output And Exit Codes

- Default `run` output is a compact per-request summary with request numbering, status, duration, and response size.
- `--verbose` prints full request and response details, including headers and bodies.
- `run` returns `0` when all selected files complete successfully.
- `run` returns `1` if any file fails.
- `validate` returns `0` when all files validate successfully.
- `validate` returns `1` if any file fails validation.
- Invalid CLI usage returns `2`.
- Assertion failures always return `1`.
- With `--fail-http`, HTTP status `>= 400` is treated as command failure.

## Not Supported

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

## Development

Build:

```bash
make build
```

Run tests:

```bash
make test
```

Current tests cover:

- Parser behavior
- Variable resolution and precedence
- External body files
- Request directives
- Response assertions across all supported subjects and operators, including parse errors and runtime failures
- Redirect and cookie behavior
- Timeout behavior
- `--name` selection
- Multi-file CLI execution
- File-level concurrency via `--jobs`
