# httprun

English README: [`README.md`](./README.md)

`httprun` 是一个用于执行 `.http` 文件的 Go CLI，目前支持 JetBrains `.http` 文件格式的一个收敛子集，聚焦常见 HTTP 请求工作流，不追求与 `ijhttp` 或 IDE 脚本能力完全兼容。

## 一眼看清能力

| 类别 | 支持内容 |
| --- | --- |
| 命令 | `run`、`validate` |
| 文件执行 | 单次命令执行多个 `.http` 文件，支持 `--jobs` 文件级并发 |
| 请求组织 | `###` 分段、多请求、`# @name` 命名请求 |
| 请求方法 | `GET`、`POST`、`PUT`、`PATCH`、`DELETE`、`HEAD` |
| 变量 | 文件变量、env 文件、CLI `--var`、内置变量 `{{$uuid}}` / `{{$timestamp}}` |
| 请求内容 | URL、Header、内联 Body、外部 Body 文件 `< path` |
| 请求选项 | `@timeout`、`@connection-timeout`、`@no-redirect`、`@no-cookie-jar` |
| 断言 | `# @assert <expression>`，支持 `status`、`body`、`json.<path>`、`header.<name>` |
| 输出 | 默认紧凑摘要，`--verbose` 查看完整请求/响应细节 |
| 执行模型 | 单文件内串行，多文件间并发，cookie jar 在单文件内共享 |

## 快速开始

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

## 常用命令

顶层用法：

```text
Usage:
  httprun run [flags] <file.http> [more.http ...]
  httprun validate [flags] <file.http> [more.http ...]
```

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
| `--name <request>` | 只执行指定名称的请求 |
| `--env <env>` | 从 `http-client.env.json` 和 `http-client.private.env.json` 加载变量 |
| `--var key=value` | 覆盖变量，可重复传入 |
| `--jobs <n>` | 文件级并发数，默认 `1` |
| `--timeout <duration>` | 默认请求超时，默认 `30s` |
| `--verbose` | 打印完整请求和响应细节 |
| `--fail-http` | 遇到 HTTP 状态码 `>= 400` 时返回非零退出码 |

### `validate`

校验一个或多个 `.http` 文件，但不发起真实请求。

```bash
httprun validate examples/demo.http
httprun validate --name ping examples/demo.http
httprun validate --jobs 8 a.http b.http
httprun validate --name ping --env dev path/to/demo.http
```

支持参数：`--name`、`--env`、`--var`、`--jobs`。

## 最常用的写法

### 多请求

```http
###
GET https://example.com/health

###
POST https://example.com/items
Content-Type: application/json

{"name":"demo"}
```

### 命名请求

```http
###
# @name login
POST https://example.com/login
Content-Type: application/json

{"user":"demo","pass":"secret"}
```

执行单个命名请求：

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

### 环境变量文件

`httprun` 会在 `.http` 文件同目录查找：

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

变量优先级：

1. CLI `--var`
2. `http-client.env.json`
3. `http-client.private.env.json`
4. 文件内变量，例如 `@base = ...`
5. 内置变量

### 内置变量

当前支持：

- `{{$uuid}}`
- `{{$timestamp}}`

```http
POST https://example.com/events
X-Request-Id: {{$uuid}}

{"createdAt":"{{$timestamp}}"}
```

### 外部 Body 文件

```http
@payload = payload.json

###
POST https://example.com/items
Content-Type: application/json

< {{payload}}
```

Body 文件路径相对于 `.http` 文件所在目录解析，文件内容中的变量也会继续替换。

## 请求指令

请求指令是请求行之前的注释行。

```http
###
# @timeout 50s
# @connection-timeout 2s
# @no-redirect
GET {{base}}/slow
```

请求行类指令支持内联写法：

```http
###
# @no-redirect GET {{base}}/redirect
```

| 指令 | 作用 |
| --- | --- |
| `# @timeout 50s` | 覆盖单请求超时 |
| `# @connection-timeout 2s` | 覆盖连接超时 |
| `# @no-redirect` | 禁止自动跟随重定向 |
| `# @no-cookie-jar` | 不把响应 cookie 写入共享 jar |

## 断言

断言也是请求行之前的注释指令。

```http
###
# @assert status == 200
# @assert body contains "\"ok\": true"
# @assert json.data.user.name == "demo"
# @assert header.Content-Type contains "application/json"
GET {{base}}/profile
```

### 支持的 Subject 和 Operator

| Subject | Operator | 示例 |
| --- | --- | --- |
| `status` | `==`、`!=`、`>`、`>=`、`<`、`<=` | `# @assert status == 200` |
| `body` | `==`、`!=`、`contains`、`not_contains`、`exists`、`not_exists` | `# @assert body contains hello` |
| `json.<path>` | `==`、`!=`、`>`、`>=`、`<`、`<=`、`exists`、`not_exists` | `# @assert json.data.count >= 2` |
| `header.<name>` | `==`、`!=`、`contains`、`not_contains`、`exists`、`not_exists` | `# @assert header.X-Trace-Id exists` |

### 断言说明

- `@assert` 必须写在请求行之前。如果写在 body 后面，它会被当成 body 内容，而不是断言。
- `json.<path>` 使用点路径语法，数组下标使用数字片段，例如 `json.data.items.0.id`。
- JSON 比较值必须是合法 JSON。字符串要带引号，布尔值使用 `true` / `false`，数字使用 JSON 数字字面量。
- 任一断言失败时，当前文件会立刻停止执行，后续请求会被跳过。

## 执行模型

- 单个 `.http` 文件内部按顺序执行。
- 同一次命令中的多个文件可通过 `--jobs` 并发执行。
- 即使文件并发执行，输出也按输入顺序打印。
- cookie jar 在单个文件执行过程中共享。
- cookie jar 不会跨文件共享。

## 示例文件

主示例：

- [examples/demo.http](./examples/demo.http)：最小可运行示例

更多示例：

- [examples/all_methods.http](./examples/all_methods.http)：常见 HTTP 方法、变量、env 文件、外部 Body 文件
- [examples/assertions.http](./examples/assertions.http)：成功断言示例，覆盖 `status`、`body`、`json.*`、`header.*` 和多种 operator
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

## 输出和退出码

- `run` 默认输出紧凑摘要，包含请求编号、状态码、耗时和响应体大小。
- `--verbose` 会打印完整的请求和响应细节，包括头和 body。
- `run` 在所有目标文件都成功时返回 `0`。
- `run` 只要任一文件失败就返回 `1`。
- `validate` 在所有文件都校验通过时返回 `0`。
- `validate` 只要任一文件校验失败就返回 `1`。
- 非法 CLI 用法返回 `2`。
- 响应断言失败始终返回 `1`。
- 启用 `--fail-http` 后，HTTP 状态码 `>= 400` 会被视为命令失败。

## 暂不支持

- pre-request 脚本
- response handler 脚本
- 从前一个响应中提取变量
- `client.*` 之类的 JavaScript API
- WebSocket
- GraphQL 专用语法
- gRPC
- OAuth 和更高级的认证辅助能力
- multipart/form-data 语法糖
- 目录扫描和递归发现

## 开发

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
- 外部 Body 文件
- 请求指令解析
- 响应断言的全部 subject 和 operator，以及解析错误和运行时失败路径
- redirect 和 cookie 行为
- timeout 行为
- `--name` 选择逻辑
- 多文件 CLI 执行
- `--jobs` 文件级并发
