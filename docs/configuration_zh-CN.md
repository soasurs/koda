# 配置说明

[English](configuration.md)

Koda 会从 `~/.koda/koda.yaml` 读取可选的进程级配置。命令行参数优先于配置文件。
Provider、workspace、model、reasoning effort 和权限都属于 Session，因此不在该文件中
配置。

## 示例

```yaml
version: 1
server:
  address: 127.0.0.1:8080
log:
  level: info
  output: console
  # path: ~/.koda/koda.log
context:
  window_tokens: 256000
compaction:
  enabled: true
  trigger_percent: 80
  reserve_tokens: 32768
  summary_max_tokens: 8192
  retain_turns: 2
  retain_tokens: 12000
  verify: true
  rebase_interval: 5
mcp:
  servers:
    - id: exa
      name: Exa
      transport: http
      url: https://mcp.exa.ai/mcp
      read_only: true
      headers:
        x-api-key: ${EXA_API_KEY}
    - id: local-search
      name: Local search
      transport: stdio
      command: npx
      args: [-y, example-mcp-server]
      env:
        SEARCH_TOKEN: ${SEARCH_TOKEN}
```

## Server 与日志

`server.address` 必须是 loopback 地址。命令行参数 `--addr` 会覆盖它。两者都未设置时，
Koda 会尝试 `localhost:8080`，必要时回退到可用的 loopback 端口。

日志级别支持 `debug`、`info`、`warn` 和 `error`，默认为 `info`。`log.output` 支持
`console`、`file` 和 `all`；未设置时等同于 `console`。控制台诊断信息写入 stderr，
选中的监听 URL 写入 stdout。文件输出使用 `log.path`；未设置路径时使用
`~/.koda/koda.log`。Debug 日志包含操作耗时、工具名称等安全的运行时元数据，但不会
记录 Prompt、工具参数、命令输出、文件内容或凭据。

## Context 统计与 Compaction

Bundled Model metadata 会声明每个已知 Model 的 context window；custom Model override
也可以通过 Studio 或公共 API 设置同一字段。`context.window_tokens` 是缺少该 metadata
时使用的进程级 fallback，默认为 256,000。Provider 返回 usage 前，Session 的使用量保持
不可用。Studio 会将最近一次返回的 prompt 和 completion usage 相加，并按当前选中 Model
的有效窗口展示。Session 切换 Model 后，有效窗口会立即变化；used-token 值则保留最近一次
Provider measurement，直到新 Model 返回 usage。

持久化 compaction 默认启用。新 Run 开始前，Koda 会解析当前 Model 的有效 context
window；如果上一个 completed turn 达到 `trigger_percent`，或者需要提前为
`reserve_tokens` 留出空间，Koda 会尝试压缩历史。
它在 `retain_tokens` 范围内最多保留 `retain_turns` 个最近完整 turn，总结更早的 active
前缀，并将生成的 working-state snapshot 注入后续模型请求。该 snapshot 不是普通的
conversation event。

Compaction 会生成带版本的结构化 JSON。无效 draft 和无效 verification result 共用一次
修复机会。因此启用 `verify: true` 后，一次 compaction 最多调用模型三次。每经过
`rebase_interval` 代，Koda 会从一个有界 checkpoint 和后续不可变 segment summary
重建状态，以限制递归总结产生的漂移。

在 reserve 边界以下，失败的尝试会被记录，Run 可以继续；使用量增加后 Koda 会重试。
到达 reserve 边界时，失败会返回 `RESOURCE_EXHAUSTED`。设置 `enabled: false` 会停止
创建新的 compaction，但已有的持久化 snapshot 仍会提供给模型。

内部设计见[存储与 Context Compaction](architecture/storage-and-compaction_zh-CN.md)。

## MCP Server

Koda 会在启动时连接所有配置的 MCP server，并将发现的 catalog 固定到当前进程生命
周期。修改 MCP 配置后需要重启 Koda。

HTTP 配置使用 MCP Streamable HTTP。远程 endpoint 必须使用 HTTPS；只有 loopback
endpoint 可以使用明文 HTTP。Stdio 配置会直接启动 `command`，不经过 Shell，并可指定
`args`、`env` 和 `workdir`。

Header、参数和环境变量值支持 `$NAME` 和 `${NAME}` 展开。引用的变量不存在、配置的
server 无法连接、返回无效 catalog 或与其它 catalog 冲突时，Koda 会启动失败，不会
静默删除预期能力。

工具以 `mcp__<server-id>__<tool-name>` 名称暴露给模型，结果进入模型 context 前会受到
大小限制。`ListMCPServers`、`GetMCPServer` 以及 Studio 的 Settings > MCP 页面会展示
启动时 catalog，但不会返回 HTTP header 或 stdio 环境变量值。

标记为 `read_only: true` 的 server 可用于 Plan 和 Build 模式，并且调用时无需逐次审批。
其它 MCP 工具只用于 Build 模式，每次调用都需要审批。只有在 server 的所有工具都无
副作用时才能设置 `read_only: true`。Stdio server 是以 Koda 用户权限运行的受信任本地
进程；审批不会对该进程进行沙箱隔离。

## Provider

Koda 内置以下 Provider：

| ID | API | 环境变量 |
|---|---|---|
| `anthropic` | Anthropic Messages | `ANTHROPIC_API_KEY` |
| `openai` | OpenAI Chat Completions | `OPENAI_API_KEY` |
| `openai-responses` | OpenAI Responses | `OPENAI_API_KEY` |
| `gemini` | Gemini GenerateContent | `GEMINI_API_KEY` |
| `deepseek` | DeepSeek | `DEEPSEEK_API_KEY` |

环境凭据优先于已保存凭据，并且不会写入 Koda 的 Provider 文件。客户端通过
`koda.v1.KodaService` 管理保存的凭据、自定义 endpoint、model override 和 discovery。
Model 列表只读取本地状态；只有显式调用 `RefreshModels` 才会访问网络。Provider 连接
发生变化后，之前的 discovery snapshot 会失效。

## 本地数据

Koda 将本地状态保存在 `~/.koda`：

```text
~/.koda/koda.yaml        可选的进程级配置
~/.koda/providers.json   Provider 定义和保存的凭据
~/.koda/koda.db          Session 和 ADK 对话历史
~/.koda/skills/          启动时加载的 Agent Skills
```

Provider registry 与 `koda.yaml` 分开保存，因为客户端会通过 API 更新前者，而
`koda.yaml` 只在进程启动时读取。Provider 文件只允许当前用户访问。

## Agent Skills

`~/.koda/skills` 的每个直接子目录可以包含一个 Agent Skill，其 `SKILL.md` name 必须
与目录名一致。Koda 在启动时加载一次 catalog。Agent 使用 `load_skill` 加载选中的定义，
使用 `read_skill_resource` 读取其中列出的 UTF-8 资源。

新增、删除或修改 skill 后需要重启 Koda。目录不存在时按空 catalog 处理。无效 skill
会被记录并跳过；如果目录本身无法加载，Koda 会记录错误并使用空 catalog 继续启动。
`ListSkills`、`GetSkill` 和 Studio 的 Settings > Skills 页面会展示固定的启动快照。
