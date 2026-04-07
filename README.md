# httprun

中文文档：[`README.zh-CN.md`](./README.zh-CN.md)

## NAME

`httprun` - command-line tool for running `.http` files

## SYNOPSIS

```text
httprun run [flags] <file.http> [more.http ...]
httprun validate [flags] <file.http> [more.http ...]
```

## DESCRIPTION

`httprun` runs JetBrains-style `.http` files and supports the subset of syntax most commonly used in practice.

It is meant for keeping HTTP requests, variable substitution, and simple assertions in files and running them from the command line. It is not a full replacement for `ijhttp`, and it does not support IDE scripting features.

## QUICK START

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

Notes:

- `run` sends real HTTP requests.
- `validate` checks whether the file can be parsed correctly, but does not send requests.
- On success, the default output shows status, duration, and response size for each request.

## COMMANDS

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
| `--env <env>` | Read variables from `http-client.env.json` and `http-client.private.env.json` |
| `--var key=value` | Override variables, repeatable |
| `--jobs <n>` | Number of `.http` files to run at the same time, default `1` |
| `--timeout <duration>` | Default request timeout, default `30s` |
| `--verbose` | Print full request and response details |
| `--fail-http` | Return non-zero when HTTP status is `>= 400` |

### `validate`

Check whether one or more `.http` files are valid, without sending real requests.

```bash
httprun validate examples/demo.http
httprun validate --name ping examples/demo.http
httprun validate --jobs 8 a.http b.http
httprun validate --name ping --env dev path/to/demo.http
```

Supported flags: `--name`, `--env`, `--var`, `--jobs`.

## HTTP FILE FORMAT

This section shows the most common `.http` file patterns.

### Multiple requests

```http
###
GET https://example.com/health

###
POST https://example.com/items
Content-Type: application/json

{"name":"demo"}
```

Use `###` to separate requests. A single file can contain multiple requests.

### Named requests

```http
###
# @name login
POST https://example.com/login
Content-Type: application/json

{"user":"demo","pass":"secret"}
```

Run only that request:

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
- Variable references use `{{key}}`.
- Directive comments such as `# @name`, `# @assert`, and `# @timeout` are different from variable declarations such as `@base = ...`.
- `@name = foo` is still treated as a normal variable, but that style is not recommended because it makes the file harder to read.

### Built-in variables

Currently supported:

- `{{$uuid}}`
- `{{$timestamp}}`

```http
POST https://example.com/events
X-Request-Id: {{$uuid}}

{"createdAt":"{{$timestamp}}"}
```

### External request body files

```http
@payload = payload.json

###
POST https://example.com/items
Content-Type: application/json

< {{payload}}
```

Notes:

- `< path` means "load the request body from a file".
- The path is resolved relative to the `.http` file directory.
- Variable interpolation also applies to the loaded file content.

### Placement rules

- File variables, request names, request directives, and assertions must appear before the request line.
- Headers go after the request line and before the first blank line.
- The request body starts after the first blank line.
- Anything written after the request body is still treated as body content, not as a new directive.

## REQUEST DIRECTIVES

Request directives are comment lines placed before the request line.

```http
###
# @timeout 50s
# @connection-timeout 2s
# @no-redirect
GET {{base}}/slow
```

Some directives can also share the same line as the request:

```http
###
# @no-redirect GET {{base}}/redirect
```

| Directive | Meaning |
| --- | --- |
| `# @timeout 50s` | Override the timeout for the current request |
| `# @connection-timeout 2s` | Override the connection timeout for the current request |
| `# @no-redirect` | Do not follow redirects automatically |
| `# @no-cookie-jar` | Do not write cookies from this response back into the shared cookie store |

## ASSERTIONS

Assertions also appear before the request line.

```http
###
# @assert status == 200
# @assert body contains hello
# @assert json.data.user.name == "demo"
# @assert header.Content-Type contains "application/json"
GET {{base}}/profile
```

### Supported checks

| What to check | Operators | Example |
| --- | --- | --- |
| `status` | `==`, `!=`, `>`, `>=`, `<`, `<=` | `# @assert status == 200` |
| `body` | `==`, `!=`, `contains`, `not_contains`, `exists`, `not_exists` | `# @assert body contains hello` |
| `json.<path>` | `==`, `!=`, `>`, `>=`, `<`, `<=`, `exists`, `not_exists` | `# @assert json.data.count >= 2` |
| `header.<name>` | `==`, `!=`, `contains`, `not_contains`, `exists`, `not_exists` | `# @assert header.X-Trace-Id exists` |

### Assertion rules

- `@assert` must appear before the request line.
- If you place `@assert` after the body, it is treated as body content.
- `json.<path>` uses dot notation, with numeric segments for array indexes, for example `json.data.items.0.id`.
- JSON comparison values must be valid JSON. Strings must be quoted, booleans must be `true` or `false`, and numbers must use JSON number syntax.
- If any assertion fails, execution of the current file stops immediately and later requests in that file are skipped.

## FILES

`httprun` looks for these files in the same directory as the `.http` file:

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

Variable precedence from highest to lowest:

1. CLI `--var`
2. `http-client.env.json`
3. `http-client.private.env.json`
4. File variables such as `@base = ...`
5. Built-in variables

## EXECUTION RULES

- Requests inside one `.http` file run sequentially.
- Multiple files in one command can run concurrently with `--jobs`.
- Output is still printed in input order even when files run concurrently.
- Requests in the same file share cookies.
- Cookies are not shared across different files.

## EXIT STATUS

- `run` returns `0` when all selected files complete successfully.
- `run` returns `1` if any file fails.
- `validate` returns `0` when all files validate successfully.
- `validate` returns `1` if any file fails validation.
- Invalid CLI usage returns `2`.
- Assertion failures always return `1`.
- With `--fail-http`, HTTP status `>= 400` is treated as command failure.

## EXAMPLES

Primary example:

- [`examples/demo.http`](./examples/demo.http): minimal runnable example

Additional examples:

- [`examples/all_methods.http`](./examples/all_methods.http): common HTTP methods, variables, environment files, external request body files
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

## LIMITATIONS

`httprun` is better suited to straightforward request files with variable substitution and assertions than to script-heavy workflows. The following are not supported:

- Pre-request scripts
- Response handler scripts
- Extracting variables from previous responses
- JavaScript APIs such as `client.*`
- WebSocket
- GraphQL-specific syntax
- gRPC
- OAuth and advanced auth helpers
- Multipart/form-data shorthand
- Directory scanning and recursive discovery

## DEVELOPMENT

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
- External request body files
- Request directives
- Response assertions across all supported checks and operators, including parse errors and runtime failures
- Redirect and cookie behavior
- Timeout behavior
- `--name` selection
- Multi-file CLI execution
- File-level concurrency via `--jobs`
