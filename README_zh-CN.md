# koda

`koda` 是一个本地 coding agent 服务，使用 Go、Protocol Buffers、Connect RPC 和
[`github.com/soasurs/adk`](https://github.com/soasurs/adk) 构建。它为客户端提供
流式 agent turn、workspace 工具、操作审批、结构化提问、Provider 配置和持久化对话
历史等运行时能力。

[English](README.md)

Koda 当前只提供无界面的本地服务，不包含 UI。

## 启动服务

环境要求：

- Go 1.26，或 `go.mod` 声明的版本；
- 至少配置一个 Provider 的 API key。

启动 Connect API server：

```bash
go run ./cmd/koda serve
```

Koda 会先尝试 `localhost:8080`。若端口已被占用，它会选择另一个 loopback 端口并
输出实际地址。也可以显式指定端口：

```bash
go run ./cmd/koda serve --addr 127.0.0.1:8787
```

服务只接受 loopback 地址，并且不会打开浏览器。

## Provider 与本地数据

Koda 内置以下 Provider：

| ID | API | 环境变量 |
|---|---|---|
| `anthropic` | Anthropic Messages | `ANTHROPIC_API_KEY` |
| `openai` | OpenAI Chat Completions | `OPENAI_API_KEY` |
| `openai-responses` | OpenAI Responses | `OPENAI_API_KEY` |
| `gemini` | Gemini GenerateContent | `GEMINI_API_KEY` |
| `deepseek` | DeepSeek | `DEEPSEEK_API_KEY` |

环境变量中的凭据优先于已保存的凭据，并且不会被复制到 Koda 配置文件中。客户端可以
通过 `koda.v1.KodaService` 管理自定义 endpoint 和模型 override。

Koda 将状态保存在 `~/.koda`：

```text
~/.koda/providers.json   Provider 定义与凭据
~/.koda/koda.db          Session 与 ADK 对话历史
```

Provider 文件仅允许当前用户访问。模型列表只读取本地状态；只有客户端显式调用
`RefreshModels` 时才会访问网络。Provider 连接发生变化后，之前发现的模型 snapshot
会失效。

## Agent Run

`Run` 通过 server stream 执行一个多模态用户 turn。输入可以按顺序包含文本、HTTPS
图片 URL，或带 MIME type 的内联图片 bytes。流中有四种 frame：

- `Event`：模型增量或完整对话事件；
- `ToolApproval`：等待用户批准的操作；
- `QuestionPrompt`：agent 发起的结构化提问；
- `RunCompleted`：turn 成功提交后的完成信号。

每个 Session 独立选择 Provider、Model、reasoning effort、workspace 和权限策略。同一
Session 的 Run 会串行执行；如果已经提交的 turn 无法通过 `RunCompleted` 被确认，历史
会回滚。

## 工具与权限

Plan agent 可以读取文件、列目录、搜索内容、查找文件、提问，并可通过 `run_shell`
执行一条白名单内的只读 Git 命令。其他命令和会修改仓库的 Git 操作都会被拒绝。

Build agent 额外提供整文件创建和写入、Hashline 编辑，以及支持任意命令语法的
`run_shell`。Build 模式的 Shell 执行仍受 Session 的 Shell 审批策略控制。

文件访问按 Session 配置：

| 级别 | Workspace 内读取 | Workspace 内写入 | Workspace 外访问 |
|---|---:|---:|---:|
| `WORKSPACE_READ` | 允许 | 审批 | 审批 |
| `WORKSPACE_WRITE` | 允许 | 允许 | 审批 |
| `UNRESTRICTED` | 允许 | 允许 | 允许 |

工具会先解析符号链接，再判断路径是否位于 workspace 内。Shell 权限独立配置，因为
不受限的进程实际上可以访问整个文件系统。

`read_file` 和 `search_text` 会返回内容 revision 与 `LINE:HASH` 锚点；`edit_file`
在原子应用修改前再次校验它们。可预测的文件写入会在审批帧和结果帧中携带结构化 diff。

## 开发

API 的源文件是 [`proto/koda/v1/service.proto`](proto/koda/v1/service.proto)。`gen/`
下的生成文件需要提交，但不能手工修改。

修改 Go 代码后运行：

```bash
gofmt -w .
go build ./...
go vet ./...
go test ./...
go test -cover ./...
go test -race ./...
git diff --check
```

修改 Protocol Buffer 契约后，先运行 `buf format -w`、`buf lint`、`buf build` 和
`buf generate`，再执行上述 Go 检查。

贡献者需要遵循的仓库规则见 [AGENTS.md](AGENTS.md)。

## License

Apache License 2.0，详见 [LICENSE](LICENSE)。
