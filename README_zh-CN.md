# koda

> **Warning: 早期开发阶段 — 可能存在破坏性变更、功能不完整和各种粗糙之处。**

koda 是一个用 Go 编写的终端 AI 编程助手，灵感来自 Claude Code 和 Aider。它提供丰富的 TUI 界面，支持与大语言模型进行交互式对话，内置文件编辑、Shell 执行、代码搜索和 Git 工具，帮助你在终端中更高效地编程。

## 特性

- **交互式 TUI** — 基于 Bubbletea 的界面，支持可滚动消息历史、多行输入和流式响应
- **单次模式** — 通过命令行参数传入 prompt，实现快速非交互式调用
- **多供应商 LLM 支持** — 支持 Anthropic (Claude)、OpenAI (GPT) 和 Google Gemini，可在运行时通过 `/connect` 和 `/model` 命令切换
- **9 个内置工具** 供 Agent 调用：
  | 工具 | 说明 |
  |------|------|
  | `read_file` | 读取文件内容（支持行范围） |
  | `write_file` | 覆写文件 |
  | `create_file` | 创建新文件（已存在则拒绝） |
  | `list_directory` | 列出目录内容 |
  | `grep_search` | 正则表达式搜索文件内容 |
  | `find_files` | 基于 glob 模式查找文件 |
  | `run_shell` | 执行 Shell 命令（60 秒超时） |
  | `git_status` | 显示工作区状态 |
  | `git_diff` | 显示差异（暂存或未暂存） |
- **Build 与 Plan 模式** — 按 `Tab` 在完整工具访问（Build）和只读子集（Plan，用于架构分析）之间切换
- **思考级别** — 按 `Ctrl+T` 在关闭 / 低 / 中 / 高四档扩展思考间切换
- **会话持久化** — 基于 SQLite 的对话历史，存储于 `~/.koda/sessions.db`；通过 `/sessions` 浏览和恢复
- **上下文压缩** — 自动滑动窗口压缩保持上下文可控；也可通过 `/compact` 手动触发
- **可折叠工具输出** — 长工具结果默认折叠，按 `x` 展开
- **项目感知** — 读取工作区的 `AGENTS.md` 以获取项目特定指令
- **安全模式** — 可通过 `--safe` 为 `run_shell`、`write_file`、`create_file` 等有副作用的工具调用启用执行前确认

## 环境要求

- **Go 1.26+**
- **C 编译器**（`go-sqlite3` 需要 CGo）— macOS 上执行 `xcode-select --install` 即可

## 安装

```bash
# 克隆仓库
git clone https://github.com/soasurs/koda.git
cd koda

# 构建
go build ./cmd/koda

# 或安装到 $GOPATH/bin
go install ./cmd/koda
```

## 使用方法

### 配置 LLM 供应商

导出对应的 API Key 环境变量：

```bash
export ANTHROPIC_API_KEY=sk-...    # Anthropic（默认）
export OPENAI_API_KEY=sk-...       # OpenAI
export GEMINI_API_KEY=...          # Google Gemini
```

也可以在 TUI 中通过 `/connect` 斜杠命令持久化保存 API Key。

### 交互模式

```bash
koda
```

### 单次模式

```bash
koda "列出当前目录下的文件"
```

### 命令行参数

| 参数 | 说明 |
|------|------|
| `--provider` | LLM 供应商：`anthropic`、`openai` 或 `gemini` |
| `--model` | 模型名称（如 `claude-sonnet-4-5`、`gpt-4o`） |
| `--no-session` | 禁用 SQLite 会话持久化（仅内存） |
| `--safe` | 在有副作用的工具调用执行前要求确认 |

### 环境变量

| 变量 | 用途 |
|------|------|
| `KODA_PROVIDER` | 默认供应商 |
| `KODA_MODEL` | 默认模型名称 |
| `KODA_SAFE_MODE` | 默认启用安全模式（`true` / `false`） |
| `KODA_BASE_URL` | OpenAI 兼容端点的自定义 Base URL |
| `ANTHROPIC_API_KEY` | Anthropic API Key |
| `OPENAI_API_KEY` | OpenAI API Key |
| `GEMINI_API_KEY` | Google Gemini API Key |

### 快捷键

| 按键 | 操作 |
|------|------|
| `Enter` | 发送消息 |
| `Ctrl+Enter` | 插入换行 |
| `Tab` | 切换 Build / Plan 模式 |
| `Ctrl+T` | 切换思考级别（关闭 / 低 / 中 / 高） |
| `Esc` (x2) | 取消正在运行的 Agent |
| `[` / `]` | 在消息间导航 |
| `x` | 展开 / 折叠工具输出 |
| `PgUp` / `PgDn` | 滚动视口 |

### 斜杠命令

| 命令 | 操作 |
|------|------|
| `/connect` | 选择 LLM 供应商并输入 API Key |
| `/model` | 从供应商实时模型列表中选择模型 |
| `/sessions` | 浏览并恢复历史会话 |
| `/help` | 显示命令、快捷键和安全模式提示 |
| `/new` | 创建新会话 |
| `/compact` | 压缩当前会话上下文 |
| `/undo` | 移除最后一轮用户输入及其后续消息 |

## 项目结构

```
koda/
├── cmd/koda/main.go              # CLI 入口
├── internal/
│   ├── agent/
│   │   ├── setup.go              # LLM + 工具 + 会话组装
│   │   ├── runtime.go            # Build/Plan Agent，会话生命周期
│   │   ├── models.go             # 按供应商实时查询模型列表
│   │   ├── session_catalog.go    # 会话元数据（SQLite + 内存）
│   │   ├── prompt.md             # 系统提示词（嵌入）
│   │   └── prompt_plan.md        # Plan 模式系统提示词
│   ├── config/config.go          # 环境变量 / 命令行 / 文件配置
│   ├── tools/                    # 工具实现
│   │   ├── file.go               # 读取、写入、创建、列目录
│   │   ├── shell.go              # run_shell
│   │   ├── search.go             # grep_search、find_files
│   │   └── git.go                # git_status、git_diff
│   └── tui/                      # Bubbletea TUI
│       ├── app.go                # 主模型、流式处理、渲染
│       ├── commands.go           # 斜杠命令处理
│       ├── messages.go           # 消息类型与 tea.Msg 定义
│       └── styles.go             # lipgloss 样式
├── go.mod
└── AGENTS.md                     # 编程 Agent 指令
```

## 许可证

[Apache 2.0](LICENSE)
