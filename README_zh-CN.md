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
- 历史事件与 Undo Connect handlers，以及协议级测试。
- 具备 workspace 感知能力的文件、搜索、Git 和 Shell 工具，实现了 Hashline 编辑和结构化文件 diff。
- Plan 和 Build agent 可用的 `ask_questions` 工具，包含类型化前端问题、答案校验和取消语义。
- Session 级文件/Shell 权限契约，以及支持取消的进程内 approval broker。
- Proto/ADK 多模态输入和事件转换，包含结构化工具结果/file diff，以及 fake `TurnRunner` test seam。

下一阶段将构造并缓存 agent，把工具与 approval/question broker 接入流式 Run。
当前架构决策和开发顺序见
[AGENTS.md](AGENTS.md)。

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
internal/tools/                           Workspace 感知的 coding tools
internal/permission/                      Session 权限策略
buf.yaml / buf.gen.yaml                  lint 和代码生成配置
```

计划中的包包括 `internal/agent` 和 `cmd/koda`。

## API 模型

### Run

`Run` 通过 server stream 执行一个用户 turn。流中可能包含：

- `Event`：增量文本/reasoning，或者完整的持久化事件。
- `ToolApproval`：等待用户批准的文件、Git 或 Shell 工具调用。
- `QuestionPrompt`：等待前端返回用户答案的 `ask_questions` 调用。
- `RunCompleted`：表示 turn 成功完成的终止帧。

一个 turn 从一条多模态用户输入开始，包含模型产生的所有工具调用、工具结果和后续
模型调用，直到模型返回一条不再请求工具调用的最终响应。

Run 输入支持有序的文本和图片 part。图片可以通过 HTTPS URL 提供，也可以通过带
MIME type 的原始 bytes 提供；Connect JSON 会将 protobuf bytes 表示为 base64。

### Session

Provider 和 Model 的选择属于 Session，而不是全局配置。Session 保存：

- Provider ID 和 Model ID；
- Provider-specific reasoning effort；
- 文件访问级别；
- Shell 访问级别；
- 工作目录；
- 标题和时间戳。

`RunRequest` 只携带 Session ID、用户输入和 build/plan 模式。

创建或更新 Session 时，workdir 会归一化为存在且解析过符号链接的绝对目录。Provider、
Model 和 reasoning effort 只根据本地 Model Catalog 校验，不会隐式触发网络发现。

### Session Store

默认数据库位于 `~/.koda/koda.db`。Koda metadata 存在 `koda_sessions`，ADK 的
历史表以独立前缀存放在同一个 SQLite 数据库中。创建 Koda session 时不会创建空的
ADK ledger；它会在第一次 Run 前创建，从而无需复制 ADK 的存储写入，也能保持
metadata 创建原子。完整 turn 通过支持 context 取消的进程内 session lock 串行化。

### 工具与审批

文件访问是 Session 级配置，自动放行能力分为三个等级：

| 级别 | Workspace 内读取 | Workspace 内写入 | Workspace 外访问 |
|---|---:|---:|---:|
| `WORKSPACE_READ` | 允许 | 审批 | 审批 |
| `WORKSPACE_WRITE` | 允许 | 允许 | 审批 |
| `UNRESTRICTED` | 允许 | 允许 | 允许 |

Shell 权限独立配置：默认每次都需要审批；放开后有任意进程执行和实际上的全文件系统访问能力。

Plan agent 提供 `read_file`、`list_directory`、`search_text`、`find_files`、只读白名单
`git` 和 `ask_questions`；Build agent 额外提供 `write_file`、`create_file`、基于 Hashline
的 `edit_file` 和 `run_shell`。文件工具会先解析符号链接再判断是否位于 workspace 内。`read_file` 和
`search_text` 返回文件 revision 与 `LINE:HASH` 锚点；`edit_file` 会在原子写入前校验两者。

审批可以同步暂停工具调用。未来的 Run runtime 会在可预测的文件修改场景发送带 proposed
structured diff 的 `ToolApproval` 帧，客户端通过 `ResolveToolApproval` 返回批准或拒绝。
Pending approval 仅存在于当前进程，绑定活跃 Run，支持 context 取消，并会在完成后清理。
`ask_questions` 在工具内部等待；前端提交的答案成为正常持久化的 ToolResult，
QuestionPrompt frame 本身只用于瞬态 UI。

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
go test -cover ./...
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
go test -cover ./...
```

生成器以 Go tool dependency 的形式固定在 `go.mod` 中。`gen/` 下的生成文件需要提交，
但不能手工修改。

## 路线图

当前实现顺序：

1. 可缓存的 build/plan agents，以及 approval/question interaction 的接入。
2. 流式 Run、进程生命周期和端到端测试。

## License

Apache License 2.0，详见 [LICENSE](LICENSE)。
