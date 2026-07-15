# 安全与演进

[English](security-and-evolution.md) · [架构导航](../architecture_zh-CN.md)

Koda 会暴露强大的本地能力。其安全模型依赖狭窄的网络边界、显式 Session 权限、谨慎的
路径分类，以及对凭据和诊断信息的严格处理。

## 网络边界

`koda serve` 和 `koda studio` 只绑定 loopback。HTTP 层会拒绝非 loopback Host 和非本地
浏览器 Origin。只绑定 loopback 仍不充分，因为 DNS rebinding 或恶意 Web origin 可能
借助用户浏览器访问本地 coding 能力。

当前架构不提供身份认证或多用户隔离。要让 Koda 支持远程访问，必须显式设计 identity、
authentication、authorization、transport security 和 tenancy；仅修改 listener 检查不
构成受支持的远程模式。

## 文件系统与进程信任

文件系统工具会将解析符号链接后的 target 与 Session workspace 比较。审批模型只保护
这些工具实现的操作，不是操作系统沙箱。

不受限通用 Shell 可以读写整个文件系统、启动进程、访问网络并检查环境，其实际权限比
较窄的 File access 设置更大。Stdio MCP server 同样是以 Koda 用户身份运行的受信任本地
进程。之后审批 MCP tool call 不会对 server 的启动或实现行为进行沙箱隔离。

只有在 MCP server 暴露的所有工具都无副作用时才能标记为 read-only。通用 MCP schema
不足以支持可靠的 effect inference，因此 Koda 信任该配置声明。

## 凭据处理

Provider 凭据来自环境变量或私有 Provider registry。环境凭据覆盖已保存值且不会持久化。
Provider Base URL 禁止 user-info credential。Discovery 和模型请求将凭据放在 header，而
不是 URL 中，使 URL 可以安全地出现在常规诊断路径。

Provider registry 和数据库文件只允许当前用户访问。公共 Provider 与 MCP API 会返回
metadata，但不会返回解析后的凭据、header 或 stdio 环境变量值。

## 日志与错误

日志可以包含 request、Session、turn、tool、Model、duration、token count、capability
kind 和 scope 等运行 metadata，但不能包含：

- Prompt 或模型内容；
- 工具参数或输出；
- 命令输出；
- 文件内容或拟议内容；
- API key、Authorization header 或其它凭据。

Server 会在传输边界把预期的取消、deadline、配置和容量条件映射为 Connect code。内部
详细原因可以包装用于诊断，但包含凭据的 request material 不能进入错误字符串。工具
审批拒绝是模型可见的 handled result，而不是内部 Server failure。

## 测试接缝

窄化边界让高风险行为无需真实模型也能测试：

- `TurnRunner` 向 Run 测试提供确定的 event sequence；
- Agent 测试可以替换 Provider model factory；
- MCP connection 和 transport 可以使用 fake；
- clock、ID、title 和 compactor function 有聚焦的注入点；
- Server integration test 会结合真实 Store 行为测试 Proto 转换和流式输出。

修改以下不变量时需要增加聚焦回归测试：

- per-session 串行化和等待期间取消；
- runtime 或 transport 失败后的持久化 failed/interrupted Turn 恢复；
- 同轮并发工具的 frame publication；
- 过期 Provider revision 和 compaction generation 的拒绝；
- symlink 和 future-path scope 分类；
- Host 与 Origin 拒绝；
- API、日志、URL 和错误中的 secret 排除。

完整 Go pipeline 包含 build、vet、普通测试、coverage 和 race test，因为并发本身是存储
与流式输出契约的一部分。

## 扩展规则

- 新增 RPC 时，先修改 Proto source 并重新生成 binding，在 Server 边界转换，生成类型
  不得进入核心包。
- 新增 Session 设置时，需要更新 domain type、migration、Proto conversion、validation，
  以及它影响的所有 Agent cache key 或 runtime decision；同时决定如何与 in-flight Run
  串行化。
- 新增 Provider 时，如果 wire semantic 不同，应定义独立 adapter type，增加 bundled
  model metadata，在支持时实现显式 discovery，并保持 revision 和凭据规则。
- 新增工具时，需要选择适用 mode，确定 capability kind 与 scope，解析全部 target，限制
  模型可见输出，准确描述审批，并在修改前重新校验状态。
- 新增 Run frame 时，需要定义它是瞬态还是持久化状态，与其它 frame 串行发布，明确取消
  行为，并避免把 Event 复制成另一个对话事实来源。
- 新增持久化历史变换时，需要一起设计展示历史、模型投影、undo、持久化状态、并发、
  migration 和失败恢复。只有一个 summary 字段并不是完整的持久化设计。

## 可能的演进方向

以下是设计问题，不是已实现功能：

- 使用 Provider-specific context window budget 替代单一进程级值；
- 客户端重连后持久恢复 approval 或 question；
- 为单个 MCP tool 增加显式 capability metadata；
- 独立发布的客户端与 Server 之间进行版本协商；
- Session 和 compaction generation 的导出与迁移格式；
- 带身份认证和隔离的远程或多用户部署模型。

任何此类变更都必须保持或有意替换当前所有权、确认和安全不变量，而不能局部绕过它们。
