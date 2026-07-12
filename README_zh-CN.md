# koda

`koda` 是一个使用 Go 编写、运行在本机的 coding agent 服务。项目正在围绕版本化
Protocol Buffer 契约、Connect RPC 和
[`github.com/soasurs/adk`](https://github.com/soasurs/adk) agent 框架重新实现。

[English](README.md)

## 当前状态

仓库正处于重写阶段，目前还没有可运行的服务端或 CLI。

已经完成：

- `koda.v1.KodaService` Protocol Buffer 契约。
- 生成的 Go 和 Connect bindings。
- 多模态 Run 输入，以及流式事件、工具审批和完成帧定义。
- Provider、Model、Session、历史事件和 Undo API 契约。
- 可持久化且并发安全的 Provider Registry。
- 内置模型目录，以及 Anthropic、OpenAI-compatible、Gemini 和 DeepSeek
  的远程模型发现与 last-known-good snapshot。
- 基于 SQLite 的 Session Store，负责 Koda session metadata 和 ADK 对话历史。
- Provider 和 Model 的 Connect handlers，以及协议级测试。
- Session CRUD Connect handlers，以及协议级测试。

下一阶段将实现 Proto/ADK 输入和事件转换，以及 fake-runtime test seam。当前架构决策和开发顺序见 [AGENTS.md](AGENTS.md)。

## 架构

```text
Provider Registry ─┐
Session Store ─────┼──> Agent Runtime ──> Connect Server ──> 本地客户端
Tools + Prompts ───┘
```

当前源码结构：

```text
proto/koda/v1/service.proto              API 契约源文件
gen/koda/v1/                             生成的 Go/Connect bindings
internal/provider/                       Provider Registry 和模型目录
internal/server/                          Connect handlers
internal/store/                          SQLite 生命周期和 Session Catalog
buf.yaml / buf.gen.yaml                  lint 和代码生成配置
```

计划中的包包括 `internal/agent`、`internal/tools` 和 `cmd/koda`。

## API 模型

### Run

`Run` 通过 server stream 执行一个用户 turn。流中可能包含：

- `Event`：增量文本/reasoning，或者完整的持久化事件。
- `ToolApproval`：等待用户批准的写操作工具调用。
- `RunCompleted`：表示 turn 成功完成的终止帧。

一个 turn 从一条多模态用户输入开始，包含模型产生的所有工具调用、工具结果和后续
模型调用，直到模型返回一条不再请求工具调用的最终响应。

Run 输入支持有序的文本和图片 part。图片可以通过 HTTPS URL 提供，也可以通过带
MIME type 的原始 bytes 提供；Connect JSON 会将 protobuf bytes 表示为 base64。

### Session

Provider 和 Model 的选择属于 Session，而不是全局配置。Session 保存：

- Provider ID 和 Model ID；
- Provider-specific reasoning effort；
- Safe-mode 状态；
- 工作目录；
- 标题和时间戳。

`RunRequest` 只携带 Session ID、用户输入和 build/plan 模式。

创建或更新 Session 时，workdir 会归一化为存在的绝对目录。Provider、Model 和
reasoning effort 只根据本地 Model Catalog 校验，不会隐式触发网络发现。

### Session Store

默认数据库位于 `~/.koda/koda.db`。Koda metadata 存在 `koda_sessions`，ADK 的
历史表以独立前缀存放在同一个 SQLite 数据库中。创建 Koda session 时不会创建空的
ADK ledger；它会在第一次 Run 前创建，从而无需复制 ADK 的存储写入，也能保持
metadata 创建原子。完整 turn 通过支持 context 取消的进程内 session lock 串行化。

### 工具审批

Safe mode 可以同步暂停一个会产生修改的工具调用。服务端发送 `ToolApproval` 帧，
客户端通过 `ResolveToolApproval` 返回批准或拒绝。Pending approval 保存在当前进程，
并绑定到活跃的 Run；这与 koda 的单机部署模型一致。

## Provider Registry

Registry 将 Provider 定义、凭据、用户模型 override 和远程发现 snapshot 存储在：

```text
~/.koda/providers.json
```

目录权限为 `0700`，文件权限为 `0600`。写入过程使用临时文件和原子 rename。

内置 Provider：

| ID | API | 环境变量 |
|---|---|---|
| `anthropic` | Anthropic Messages | `ANTHROPIC_API_KEY` |
| `openai` | OpenAI Chat Completions | `OPENAI_API_KEY` |
| `openai-responses` | OpenAI Responses | `OPENAI_API_KEY` |
| `gemini` | Gemini GenerateContent | `GEMINI_API_KEY` |
| `deepseek` | DeepSeek | `DEEPSEEK_API_KEY` |

环境变量中的凭据优先于文件中保存的凭据，并且不会被复制进 Registry 文件。自定义
Provider 可以选择任意受支持的 adapter type，也可以提供 HTTP(S) Base URL。

模型列表由三层组成：

1. 经过 review 并打包进二进制的目录，为使用默认地址的内置 Provider 提供离线基线。
2. `RefreshModels` 调用 Provider API，并持久化最近一次成功发现的 snapshot。
3. Provider 的 `model_overrides` 可以添加私有模型，或者按 Model ID 覆盖元数据。

`ListModels` 只读取本地状态，不会隐式发起网络请求。刷新失败会保留最近一次成功的
snapshot。自定义 endpoint 在没有成功 snapshot 时只暴露显式 override。模型目录刷新
不会改变 Provider 的连接 revision，也不会导致缓存的 LLM client 失效。

Reasoning effort 属于具体 Model。模型目录可以声明 `minimal`、`low`、`medium`、
`high`、`xhigh`、`max` 或 `ultra` 等值；未来 Runtime 会根据 Session 选择的模型进行
校验。

## 开发

环境要求：

- Go 1.26，或 `go.mod` 声明的版本。
- Buf CLI。

验证当前仓库：

```bash
buf lint
buf build
go build ./...
go vet ./...
go test ./...
go test -race ./...
```

修改 `proto/koda/v1/service.proto` 后：

```bash
buf format -w
buf lint
buf build
buf generate
go build ./...
go vet ./...
go test ./...
```

生成器以 Go tool dependency 的形式固定在 `go.mod` 中。`gen/` 下的生成文件需要提交，
但不能手工修改。

## 路线图

当前实现顺序：

1. Proto 与 ADK 之间的输入/事件转换，以及 Runtime 测试 seam。
2. 可缓存的 build/plan agents、coding tools 和 Safe-mode approval。
3. 流式 Run、进程生命周期和端到端测试。

## License

Apache License 2.0，详见 [LICENSE](LICENSE)。
