import type { EnDictionary } from '@/app/i18n/locales/en'

export const zhCN: EnDictionary = {
  // General settings
  'settings.general.title': '通用',
  'settings.general.description': '管理 Koda Studio 的外观与对话默认设置。',
  'settings.general.appearance.title': '外观',
  'settings.general.appearance.theme': '主题',
  'settings.general.appearance.theme.system': '跟随系统',
  'settings.general.appearance.theme.light': '浅色',
  'settings.general.appearance.theme.dark': '深色',
  'settings.general.appearance.language': '语言',
  'settings.general.conversation.title': '对话',
  'settings.general.conversation.expandReasoning': '默认展开推理',
  'settings.general.conversation.expandReasoning.description':
    '一轮对话开始时展开助手推理面板。',
  'settings.general.conversation.expandToolCalls': '默认展开工具调用',
  'settings.general.conversation.expandToolCalls.description':
    '工具调用执行时展开工具调用分组。',
  'settings.general.conversation.sendShortcut': '发送快捷键',
  'settings.general.conversation.sendShortcut.enter': 'Enter',
  'settings.general.conversation.sendShortcut.shiftEnter': 'Shift + Enter',
  'settings.general.conversation.sendShortcut.commandEnter': 'Command + Enter',

  // Settings layout
  'settings.layout.title': '设置',
  'settings.layout.description': '查看进程级能力并配置本地 Koda 服务。',
  'settings.layout.nav.ariaLabel': '设置',
  'settings.layout.nav.providers': '提供方',
  'settings.layout.nav.sessions': '会话',
  'settings.layout.nav.mcp': 'MCP',
  'settings.layout.nav.skills': '技能',

  // Providers settings page
  'settings.providers.title': '提供方',
  'settings.providers.description': '配置本地 Koda 服务存储的凭据与兼容端点。',
  'settings.providers.addProvider': '添加提供方',

  // Sessions settings page
  'settings.sessions.title': '已归档会话',
  'settings.sessions.description': '恢复希望重新回到活跃项目列表的会话。',
  'settings.sessions.empty.title': '没有已归档会话',
  'settings.sessions.empty.body': '已归档会话会显示在这里。',
  'settings.sessions.restore': '恢复',

  // MCP settings page
  'settings.mcp.title': 'MCP 服务器',
  'settings.mcp.description':
    '查看从 ~/.koda/koda.yaml 加载的进程级 MCP 服务器。重启 Koda 以应用配置变更。',
  'settings.mcp.empty.title': '未配置 MCP 服务器',
  'settings.mcp.empty.body':
    '在 ~/.koda/koda.yaml 中添加 mcp.servers 条目并重启 Koda。',
  'settings.mcp.card.openAria': '打开 {name}',
  'settings.mcp.card.toolCount.one': '{count} 个工具',
  'settings.mcp.card.toolCount.other': '{count} 个工具',
  'settings.mcp.card.mode.planAndBuild': '规划 + 构建',
  'settings.mcp.card.mode.buildWithApproval': '构建(需批准)',
  'settings.mcp.details.id': 'ID',
  'settings.mcp.details.transport': '传输',
  'settings.mcp.details.agentModes': '代理模式',
  'settings.mcp.details.mode.planAndBuild': '规划与构建',
  'settings.mcp.details.mode.buildWithApproval': '构建(需批准)',
  'settings.mcp.details.target': '目标',
  'settings.mcp.details.tools.title': '工具',
  'settings.mcp.details.tools.empty': '此服务器启动时未暴露工具。',
  'settings.mcp.details.tools.mcpName': 'MCP 名称:{name}',
  'settings.mcp.transport.http': 'HTTP',
  'settings.mcp.transport.stdio': 'stdio',
  'settings.mcp.transport.unknown': '未知',

  // Skills settings page
  'settings.skills.title': '技能',
  'settings.skills.description':
    '查看本 Koda 进程启动时从 ~/.koda/skills 加载的代理技能。重启 Koda 以拾取文件系统变更。',
  'settings.skills.empty.title': '未加载技能',
  'settings.skills.empty.body': '在 ~/.koda/skills 下添加技能并重启 Koda。',
  'settings.skills.card.openAria': '打开 {name}',
  'settings.skills.details.license': '许可证',
  'settings.skills.details.compatibility': '兼容性',
  'settings.skills.details.allowedTools': '允许的工具',
  'settings.skills.details.resources': '资源',
  'settings.skills.details.instructions': '指令',

  // App shell
  'app.shell.brand': 'Koda Studio',
  'app.shell.collapseSidebarAria': '折叠侧边栏',
  'app.shell.newSession': '新建会话',
  'app.shell.new': '新建',
  'app.shell.projects': '项目',
  'app.shell.noSessions': '暂无会话',
  'app.shell.newSessionInAria': '在 {name} 中新建会话',
  'app.shell.newSessionInTitle': '在 {path} 中新建会话',
  'app.shell.settingsAria': '设置',

  // Home page
  'home.empty.title': '从一个会话开始',
  'home.empty.body': '从侧边栏创建会话。会话按本地项目分组。',
  'home.empty.configureProviders': '配置提供方',

  // Session page
  'session.notFound': '找不到会话',
  'session.empty.title': '准备就绪',
  'session.empty.body': '让 Koda 检查、规划或修改此工作区。',
  'session.compaction.continuing': '沿用现有上下文继续',
  'session.compaction.completed': '上下文已压缩 · 第 {generation} 代',
  'session.compaction.failed': '上下文压缩失败',
  'session.compaction.inProgress': '正在压缩较早的上下文…',
  'session.compaction.detail.completed':
    '{sourceTokens} 源 token · 预计压缩后 {estimatedTokens}',
  'session.compaction.detail.inProgress': '当前上下文 {contextTokens} token',
  'session.compaction.boundary.label': '较早上下文已压缩 · 第 {generation} 代',
  'session.compaction.boundary.titleGeneration': '第 {generation} 代',
  'session.compaction.boundary.titleEvents': '已摘要 {count} 个较早事件',
  'session.compaction.boundary.titleSourceTokens': '{count} 源 token',
  'session.compaction.boundary.titleEstimatedTokens':
    '压缩后预计 {count} token',
  'session.compaction.boundary.titleModel': '模型:{modelId}',

  // Shared session labels
  'session.untitled': '未命名会话',

  // Theme toggle
  'theme.toggle.label': '主题',
  'theme.toggle.ariaLabel': '主题',
}
