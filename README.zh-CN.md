# httprun

English README: [`README.md`](./README.md)

## NAME

`httprun` - 执行 `.http` 文件的命令行工具

## SYNOPSIS

```text
httprun run [flags] <file.http> [more.http ...]
httprun validate [flags] <file.http> [more.http ...]
```

## DESCRIPTION

`httprun` 用来运行 JetBrains 风格的 `.http` 文件，支持其中常用的一部分写法。

它适合把 HTTP 请求、变量替换和简单断言写进文件后统一执行，也可以先做语法和结构检查。它不是 `ijhttp` 的完整替代品，也不支持 IDE 里的脚本功能。

## QUICK START

安装：

```bash
go install github.com/smallfish/httprun/cmd/httprun@latest
httprun --help
```

最小示例：

```http
@base = https://httpbin.org

###
# @name ping
# @assert status == 200
GET {{base}}/get
Accept: application/json
```

执行：

```bash
httprun run demo.http
httprun run --name ping demo.http
httprun validate demo.http
```

说明：

- `run` 会真正发起请求。
- `validate` 只检查文件能不能被正确解析，不会发请求。
- 运行成功后，默认会看到每个请求的状态码、耗时和响应体大小。

## COMMANDS

### `run`

执行一个或多个 `.http` 文件。

```bash
httprun run examples/demo.http
httprun run --name ping examples/demo.http
httprun run --jobs 4 a.http b.http c.http
httprun run --env dev --var base=https://example.com path/to/demo.http
```

| 参数 | 作用 |
| --- | --- |
| `--name <request>` | 只执行指定名字的请求 |
| `--env <env>` | 从 `http-client.env.json` 和 `http-client.private.env.json` 读取变量 |
| `--var key=value` | 覆盖变量，可重复传入 |
| `--jobs <n>` | 同时执行多少个 `.http` 文件，默认 `1` |
| `--timeout <duration>` | 请求默认超时时间，默认 `30s` |
| `--verbose` | 打印完整的请求和响应内容 |
| `--fail-http` | 遇到 HTTP 状态码 `>= 400` 时返回非零退出码 |

### `validate`

检查一个或多个 `.http` 文件是否写得合法，但不发起真实请求。

```bash
httprun validate examples/demo.http
httprun validate --name ping examples/demo.http
httprun validate --jobs 8 a.http b.http
httprun validate --name ping --env dev path/to/demo.http
```

支持参数：`--name`、`--env`、`--var`、`--jobs`。

## HTTP FILE FORMAT

这一节说明 `.http` 文件最常用的写法。

### 写多个请求

```http
###
GET https://example.com/health

###
POST https://example.com/items
Content-Type: application/json

{"name":"demo"}
```

`###` 用来分隔请求。一个文件里可以写多个请求。

### 给请求起名字

```http
###
# @name login
POST https://example.com/login
Content-Type: application/json

{"user":"demo","pass":"secret"}
```

只执行这个请求：

```bash
httprun run --name login demo.http
```

### 文件内变量

```http
@base = https://example.com
@token = abc123

###
GET {{base}}/users
Authorization: Bearer {{token}}
```

说明：

- 文件内变量使用 `@key = value`。
- 使用变量时写成 `{{key}}`。
- `# @name`、`# @assert`、`# @timeout` 这类写在注释里的指令，和 `@base = ...` 这种变量声明不是一回事。
- `@name = foo` 仍然会被当成普通变量，但不建议这样写，不然文件读起来容易混淆。

### 内置变量

当前支持：

- `{{$uuid}}`
- `{{$timestamp}}`

```http
POST https://example.com/events
X-Request-Id: {{$uuid}}

{"createdAt":"{{$timestamp}}"}
```

### 读取外部请求体文件

```http
@payload = payload.json

###
POST https://example.com/items
Content-Type: application/json

< {{payload}}
```

说明：

- `< path` 表示从外部文件读取请求体。
- 路径相对于当前 `.http` 文件所在目录解析。
- 读取进来的文件内容也会继续做变量替换。

### 内容放在哪里

- 文件内变量、请求名字、请求前注释指令、断言，都写在请求行之前。
- 请求头写在请求行之后、第一行空行之前。
- 第一行空行之后的内容会被当成请求体。
- 如果在请求体后面继续写内容，这些内容仍然会被当成请求体，而不是新的指令。

## REQUEST DIRECTIVES

请求前的注释指令，写在请求行之前。

```http
###
# @timeout 50s
# @connection-timeout 2s
# @no-redirect
GET {{base}}/slow
```

有些指令也可以和请求行写在同一行：

```http
###
# @no-redirect GET {{base}}/redirect
```

| 指令 | 作用 |
| --- | --- |
| `# @timeout 50s` | 覆盖当前请求的超时时间 |
| `# @connection-timeout 2s` | 覆盖当前请求的连接超时时间 |
| `# @no-redirect` | 不自动跟随重定向 |
| `# @no-cookie-jar` | 不把这次响应里的 Cookie 写回共享的 Cookie 存储 |

## ASSERTIONS

断言也写在请求行之前。

```http
###
# @assert status == 200
# @assert body contains hello
# @assert json.data.user.name == "demo"
# @assert header.Content-Type contains "application/json"
GET {{base}}/profile
```

### 支持检查哪些内容

| 检查对象 | 比较方式 | 示例 |
| --- | --- | --- |
| `status` | `==`、`!=`、`>`、`>=`、`<`、`<=` | `# @assert status == 200` |
| `body` | `==`、`!=`、`contains`、`not_contains`、`exists`、`not_exists` | `# @assert body contains hello` |
| `json.<path>` | `==`、`!=`、`>`、`>=`、`<`、`<=`、`exists`、`not_exists` | `# @assert json.data.count >= 2` |
| `header.<name>` | `==`、`!=`、`contains`、`not_contains`、`exists`、`not_exists` | `# @assert header.X-Trace-Id exists` |

### 断言规则

- `@assert` 必须写在请求行之前。
- 如果把 `@assert` 写到请求体后面，它会被当成普通请求体内容。
- `json.<path>` 用点号一层层取字段，数组下标用数字，例如 `json.data.items.0.id`。
- JSON 比较值必须是合法 JSON。字符串要带引号，布尔值写成 `true` 或 `false`，数字按 JSON 数字格式书写。
- 只要有一个断言失败，当前文件就会立刻停止，后面的请求不会继续执行。

## FILES

`httprun` 会在 `.http` 文件同目录查找下面两个文件：

- `http-client.env.json`
- `http-client.private.env.json`

示例：

```json
{
  "dev": {
    "base": "https://dev.example.com",
    "token": "public-token"
  }
}
```

使用方式：

```bash
httprun run --env dev path/to/demo.http
```

变量覆盖顺序从高到低如下：

1. CLI `--var`
2. `http-client.env.json`
3. `http-client.private.env.json`
4. 文件内变量，例如 `@base = ...`
5. 内置变量

## EXECUTION RULES

- 一个 `.http` 文件里的请求按顺序执行。
- 同一次命令中的多个文件可以通过 `--jobs` 同时执行。
- 即使多个文件同时执行，输出仍然会按输入顺序打印。
- 同一个文件里的多个请求会共享 Cookie。
- Cookie 不会跨文件共享。

## EXIT STATUS

- `run` 在所有目标文件都成功时返回 `0`。
- `run` 只要任一文件失败就返回 `1`。
- `validate` 在所有文件都校验通过时返回 `0`。
- `validate` 只要任一文件校验失败就返回 `1`。
- 非法 CLI 用法返回 `2`。
- 响应断言失败始终返回 `1`。
- 启用 `--fail-http` 后，HTTP 状态码 `>= 400` 会被视为命令失败。

## EXAMPLES

主示例：

- [examples/demo.http](./examples/demo.http)：最小可运行示例

更多示例：

- [examples/all_methods.http](./examples/all_methods.http)：常见 HTTP 方法、变量、环境变量文件、外部请求体文件
- [examples/assertions.http](./examples/assertions.http)：成功断言示例，覆盖 `status`、`body`、`json.*`、`header.*` 和多种比较方式
- [examples/assertions_failure.http](./examples/assertions_failure.http)：故意失败的断言示例，展示非零退出码和后续请求被跳过
- [examples/request_options.http](./examples/request_options.http)：`@no-redirect` 和 `@no-cookie-jar`
- [examples/timeout.http](./examples/timeout.http)：请求级 `@timeout`
- [examples/http-client.env.json](./examples/http-client.env.json) 和 [examples/http-client.private.env.json](./examples/http-client.private.env.json)：环境变量文件示例

可以直接运行：

```bash
go run ./cmd/httprun run examples/demo.http
go run ./cmd/httprun run examples/assertions.http
go run ./cmd/httprun run examples/assertions_failure.http
```

## LIMITATIONS

目前更适合简单请求、变量替换和断言，不适合复杂脚本场景。下面这些能力暂不支持：

- pre-request 脚本
- response handler 脚本
- 从前一个响应中提取变量
- `client.*` 之类的 JavaScript API
- WebSocket
- GraphQL 专用语法
- gRPC
- OAuth 和更高级的认证辅助能力
- multipart/form-data 的简写写法
- 目录扫描和递归发现

## DEVELOPMENT

构建：

```bash
make build
```

运行测试：

```bash
make test
```

当前测试覆盖：

- parser 行为
- 变量解析和优先级
- 外部请求体文件
- 请求前注释指令
- 响应断言的全部检查对象和比较方式，以及解析错误和运行时失败路径
- redirect 和 cookie 行为
- timeout 行为
- `--name` 选择逻辑
- 多文件 CLI 执行
- `--jobs` 文件级并发
