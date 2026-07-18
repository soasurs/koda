# koda

`koda` 是一个使用 Go、Protocol Buffers、Connect RPC 和
[`github.com/soasurs/adk`](https://github.com/soasurs/adk) 构建的本地 coding-agent
服务。它提供持久化、权限感知的 Agent 运行时，并内置本地 Web 界面。

[English](README.md)

![Koda Studio 截图](docs/images/screenshot.png)

## 核心能力

- 支持重连、多模态输入和完整工具调用轮次的 Server-owned Agent Run；
- 带 workspace-aware 工具的 Build 和 Plan 模式；
- Session 级 Provider、Model、reasoning、workspace 和权限设置；
- Run 期间的操作审批和结构化提问；
- 支持失败/中断 Turn 状态、撤销、重试与 Model-aware context compaction 的 SQLite
  持久化对话历史；
- 内置 Anthropic、OpenAI、Gemini 和 DeepSeek adapter；
- 进程级 Agent Skills 和 MCP server；
- 内嵌 React Studio，以及供其它本地客户端使用的 Protobuf/Connect API。

Koda 是本地单进程服务，只监听 loopback 地址，并将状态保存在 `~/.koda`。

关闭或刷新 Studio 只会断开 Run event 订阅；Run 会继续在本地 Koda 进程中执行，重新进入
Session 时 Studio 会自动恢复订阅。Stop 是显式取消操作；停止 Koda 进程仍会中断 active
Run。

## 快速开始

环境要求：

- Go 1.26，或 `go.mod` 声明的版本；
- 至少一个受支持 Provider 的 API key；
- 从全新源码 checkout 构建 Studio 时需要 Node.js 24 和 pnpm 10。

设置 Provider 凭据，例如：

```bash
export ANTHROPIC_API_KEY=...
```

从全新 checkout 启动前，先构建一次内嵌 Studio 资源：

```bash
./build/studio.sh
```

启动 Koda Studio：

```bash
go run ./cmd/koda studio
```

Koda 会输出选中的 loopback URL，并使用默认浏览器打开。只启动 Connect API
server 时使用：

```bash
go run ./cmd/koda serve
```

两个命令都会先尝试 `localhost:8080`，端口被占用时回退到可用的 loopback 端口。
可以通过 `--addr 127.0.0.1:8787` 显式指定地址。

## 文档

- [配置说明](docs/configuration_zh-CN.md)介绍 `koda.yaml`、Provider、本地数据、Agent
  Skills、MCP server 和 compaction 设置。
- [架构设计](docs/architecture_zh-CN.md)介绍系统边界、Run 协议、Agent 构造、工具、
  存储、compaction 和安全模型。
- [公共 API](proto/koda/v1/service.proto)是 Protobuf 和 Connect 契约的源文件。
- [Studio](studio/README.md)介绍前端 workspace。
- [贡献者指南](AGENTS.md)包含仓库规则和完整验证命令。

## 开发

`gen/` 和 `studio/src/gen/` 下的生成文件需要提交，但不能手工修改。首次执行 Go
构建前和修改 Studio 后，需要生成被忽略的内嵌资源：

```bash
./build/studio.sh
```

随后执行 [AGENTS.md](AGENTS.md) 中的仓库检查。修改
`proto/koda/v1/service.proto` 后，需要先使用 Buf 执行格式化、lint、build 和
generate，再运行 Go 检查。

推送 `v*` tag 会触发 release workflow：构建 Studio、测试并打包原生 macOS
amd64 和 arm64 binary、生成 checksums，并创建 draft GitHub Release；annotated
tag 的正文会添加在自动生成的 release notes 前面。

## License

Apache License 2.0，详见 [LICENSE](LICENSE)。
