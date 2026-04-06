# httprun

English README: [`README.md`](./README.md)

`httprun` 是一个用于执行 `.http` 文件的 Go CLI，目前支持 JetBrains `.http` 文件格式的部分子集。

当前范围有意保持收敛：它聚焦于常见的 HTTP 请求执行场景，只支持 JetBrains `.http` 格式中的一部分能力，不声明与 `ijhttp` 或 IDE 脚本能力完全兼容。

## 当前状态

当前版本已支持：

- 单次命令执行多个 `.http` 文件
- 通过 `--jobs` 控制文件级并发
- 单个文件内部按顺序执行请求
- 使用 `###` 分隔多个请求
- 使用 `# @name` 命名请求
- 常见 HTTP 方法，如 `GET`、`POST`、`PUT`、`PATCH`、`DELETE`、`HEAD`
- 文件内变量声明，如 `@key = value`
- 在 URL、Header、内联 Body 和外部 Body 文件中进行变量替换
- 公有和私有环境变量文件
- 内置变量 `{{$uuid}}` 和 `{{$timestamp}}`
- 请求级选项：
  - `# @timeout`
  - `# @connection-timeout`
  - `# @no-redirect`
  - `# @no-cookie-jar`
- 通过 `< path/to/file` 引用外部请求体文件
- 单个文件执行过程中的 cookie jar 共享
- `run` 和 `validate` 命令

暂不支持：

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

## 安装

通过 `go install` 安装：

```bash
go install github.com/smallfish/httprun/cmd/httprun@latest
```

安装完成后可执行：

```bash
httprun --help
```

## 命令

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
```

通用示例：

```bash
httprun run --jobs 4 a.http b.http c.http
httprun run --env dev --var base=https://example.com path/to/demo.http
```

参数：

- `--name <request>`：只执行指定名称的请求
- `--env <env>`：从 `http-client.env.json` 和 `http-client.private.env.json` 中加载环境变量
- `--var key=value`：覆盖变量，可重复传入
- `--jobs <n>`：文件级并发数，默认 `1`
- `--timeout <duration>`：`run` 的默认请求超时，默认 `30s`
- `--verbose`：打印完整的请求与响应细节，包括头和 body
- `--fail-http`：当任一响应状态码 `>= 400` 时返回非零退出码

### `validate`

校验一个或多个 `.http` 文件，但不发起真实请求。

```bash
httprun validate examples/demo.http
httprun validate --name ping examples/demo.http
```

通用示例：

```bash
httprun validate --jobs 8 a.http b.http
httprun validate --name ping --env dev path/to/demo.http
```

参数：

- `--name <request>`
- `--env <env>`
- `--var key=value`
- `--jobs <n>`

## 当前支持的语法

### 多请求

使用 `###` 分隔多个请求：

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

执行指定命名请求：

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

`httprun` 会在 `.http` 文件所在目录查找：

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

示例：

```http
POST https://example.com/events
X-Request-Id: {{$uuid}}

{"createdAt":"{{$timestamp}}"}
```

### 外部 Body 文件

可以从外部文件加载请求体：

```http
@payload = payload.json

###
POST https://example.com/items
Content-Type: application/json

< {{payload}}
```

Body 文件路径相对于 `.http` 文件所在目录解析，文件内容中的变量同样会继续替换。

### 请求级选项

请求级选项通过请求前的注释指令声明。

超时覆盖：

```http
###
# @timeout 50 ms
GET {{base}}/slow
```

连接超时覆盖：

```http
###
# @connection-timeout 2 s
GET {{base}}/health
```

禁止自动跟随重定向：

```http
###
# @no-redirect
GET {{base}}/redirect
```

禁止把本次响应的 cookie 写入共享 jar：

```http
###
# @no-cookie-jar
GET {{base}}/login
```

也支持内联写法：

```http
###
# @no-redirect GET {{base}}/redirect
```

## 执行模型

执行规则如下：

- 单个 `.http` 文件内部的请求按顺序执行
- 同一次命令传入的多个文件可通过 `--jobs` 并发执行
- 即使文件并发执行，输出仍按输入文件顺序汇总
- cookie jar 在单个文件执行过程中共享
- cookie jar 不会跨文件共享

这样可以在保持单文件语义稳定的同时，提高多个独立测试文件的执行效率。

## 示例

示例文件见：[examples/demo.http](./examples/demo.http)

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

执行示例：

```bash
go run ./cmd/httprun run examples/demo.http
go run ./cmd/httprun run --name ping examples/demo.http
```

`examples/` 目录里还包含更多示范文件：

- [examples/all_methods.http](./examples/all_methods.http)：常见 HTTP 方法、变量、环境文件、外部 Body 文件；可通过 `httprun run --env dev examples/all_methods.http` 直接运行
- [examples/request_options.http](./examples/request_options.http)：`@no-redirect` 和 `@no-cookie-jar`，使用公开的 `httpbin` 接口
- [examples/timeout.http](./examples/timeout.http)：请求级 `@timeout`，使用 `httpbin /delay`
- [examples/http-client.env.json](./examples/http-client.env.json) 和 [examples/http-client.private.env.json](./examples/http-client.private.env.json)：环境变量文件示例

## 输出和退出码

`run` 默认输出为紧凑摘要格式，会展示请求序号、状态码、耗时和响应体大小。
使用 `--verbose` 可以查看完整的请求与响应细节，包括头和 body。

- `run` 在所有目标文件都成功执行时返回 `0`
- `run` 只要任一文件失败就返回 `1`
- `validate` 在所有文件都校验通过时返回 `0`
- `validate` 只要任一文件校验失败就返回 `1`
- 非法 CLI 用法返回 `2`

启用 `--fail-http` 后，HTTP 状态码 `>= 400` 会被视为命令失败。

## 开发

本地构建：

```bash
make build
```

或者直接运行：

```bash
make run-help
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
- redirect 和 cookie 行为
- timeout 行为
- `--name` 选择逻辑
- 多文件 CLI 执行
- `--jobs` 文件级并发
