# Provider 与集成

[English](providers-and-integrations.md) · [架构导航](../architecture_zh-CN.md)

Provider connection、model catalog、Agent Skills 和 MCP server 具有不同的修改与信任
模型。Koda 显式保持这些边界，使 Session 可以选择能力，而不会把进程级状态变成隐式
active 配置。

## Provider Registry

Registry 合并内置定义与用户自定义 Provider，并将可变定义保存在 `providers.json`。
内置 Provider 可以配置，但不能删除或更改成不同 adapter type。

凭据解析是有意设计的不对称过程：

- 内置环境凭据覆盖已保存凭据；
- 环境凭据绝不会写回磁盘或通过公共 API 返回；
- 更新时省略凭据字段会保留原有已保存凭据；
- 凭据只能放在 request header 中，不能进入 URL 或错误字符串。

Registry method 会返回自身 slice 和 map 的深拷贝。调用方不能绕过 registry operation
修改缓存定义或 discovery snapshot。

## Connection Revision

每个 Provider 都有 connection revision。只有 adapter type、Base URL 或已保存凭据变化
时才会推进。展示 metadata 和 model override 并不表示新的网络连接，因此不会推进它。

Revision 参与两套一致性机制：

- Agent cache entry 不能跨越连接变化继续存在；
- 长时间运行的 model discovery 只有在捕获的 revision 仍为当前值时才能提交结果。

连接变化会清除之前的 discovery snapshot，避免旧 endpoint 或旧凭据返回的响应成为新
连接的 catalog。

## Model Catalog 与 Discovery

`Catalog.List` 会合并 bundled model metadata、显式 override 和最近一次成功 discovery
snapshot，且不访问网络。因此 Agent 构造具有确定性的本地查询行为。

Model metadata 包含 Provider-specific reasoning effort 和可选的 context-window token
预算。非零的显式 override 会替换 bundled context metadata；没有声明窗口的 Model 使用
`context.window_tokens` 提供的进程级 fallback。

`Catalog.Refresh` 是显式网络操作。它使用 Provider 原生 discovery 机制，规范化响应，
并在 connection revision 仍为当前值时原子提交 snapshot。Refresh 失败会保留之前的
snapshot。没有 snapshot 的 custom endpoint 只展示显式 override。

OpenAI Chat Completions 和 OpenAI Responses 保持为不同 Provider type。即使共用 API key
和 Model 命名，它们的 request 与 response 语义仍然不同。

## Session 选择

Provider ID、Model ID 和 reasoning effort 都属于 Session。不存在进程级 active Provider。
Agent factory 会根据本地 catalog 校验选中的 Model，并在 reasoning effort 为空时使用
Model 声明的默认值。

Server 还会为 Session usage 展示解析选中 Model 的有效 context window，并在每个新 Run
前据此计算 compaction trigger 和 hard limit。

这样并发 Session 可以相互独立，Studio 设置也不会静默改变已经配置的 Session。

## Agent Skills

Skill 会在启动时加载到固定的进程 catalog。无效的单个 skill 会被跳过并记录；skill
目录加载失败时使用空 catalog。Agent factory 会渲染 catalog instruction，并添加用于
加载选中 skill 和声明的 UTF-8 resource 的 ADK tool。

Catalog instruction 参与 Runner instruction fingerprint。由于 catalog 在进程生命周期
内固定，新增或修改 skill 需要重启，而不是执行实时 cache invalidation。

## MCP 生命周期与暴露策略

MCP manager 在启动时打开所有配置的 transport 并发现工具。连接错误、nil tool、无效
名称、重复 server ID 或暴露名称冲突都会导致快速失败。不可变展示 catalog 以深拷贝返回。

模型可见名称统一为 `mcp__<server-id>__<tool-name>`，明确工具所有权并避免与内置 coding
tool 冲突。结果进入模型 context 前会受到大小限制。

显式 read-only server 会向 Plan 和 Build mode 提供无需逐次审批的工具。其它 server 只
向 Build mode 提供经过 approval wrapper 的工具。Read-only 是针对整个 server 的受信任
配置声明，不是推断结果。

HTTP transport 使用 Streamable HTTP，在 loopback 外必须使用 TLS。Stdio transport 直接
启动配置的 executable，不经过 Shell。连接前会展开环境变量，而 secret header 和环境
变量值不会进入公共 catalog。

## Studio 边界

Studio 使用与其它本地客户端相同的 Connect contract。Server 拥有持久化 Session、Event、
Provider definition、catalog 和 interaction resolution。Studio 拥有以下展示状态：

- 乐观 partial output；
- 标题生成期间根据首条输入显示的临时标题；
- 展开或折叠的 tool activity；
- 本地化时间戳和当前 theme；
- 瞬态 compaction progress。

页面刷新后，Studio 通过 Session 和 Event RPC 重建持久化状态，不能把 partial frame 保存
成对话事实。在 Run commit boundary，`RunCompleted.session` snapshot 会替换乐观 Session
状态。

目录浏览是创建 Session 前供本地客户端选择 workdir 的 service-scoped 能力。它只返回
canonical 目录名和路径，不返回文件内容，也不修改文件系统。
