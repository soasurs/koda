import type { EnDictionary } from '@/app/i18n/locales/en'

export const zhTW: EnDictionary = {
  // General settings
  'settings.general.title': '一般',
  'settings.general.description': '管理 Koda Studio 的外觀與對話預設設定。',
  'settings.general.appearance.title': '外觀',
  'settings.general.appearance.theme': '主題',
  'settings.general.appearance.theme.system': '跟隨系統',
  'settings.general.appearance.theme.light': '淺色',
  'settings.general.appearance.theme.dark': '深色',
  'settings.general.appearance.language': '語言',
  'settings.general.conversation.title': '對話',
  'settings.general.conversation.expandReasoning': '預設展開推理',
  'settings.general.conversation.expandReasoning.description':
    '一輪對話開始時展開助手推理面板。',
  'settings.general.conversation.expandToolCalls': '預設展開工具呼叫',
  'settings.general.conversation.expandToolCalls.description':
    '工具呼叫執行時展開工具呼叫群組。',
  'settings.general.conversation.sendShortcut': '傳送快速鍵',
  'settings.general.conversation.sendShortcut.enter': 'Enter',
  'settings.general.conversation.sendShortcut.shiftEnter': 'Shift + Enter',
  'settings.general.conversation.sendShortcut.commandEnter': 'Command + Enter',

  // Settings layout
  'settings.layout.title': '設定',
  'settings.layout.description': '檢視程序級能力並設定本地 Koda 服務。',
  'settings.layout.nav.ariaLabel': '設定',
  'settings.layout.nav.providers': '提供方',
  'settings.layout.nav.sessions': '對話',
  'settings.layout.nav.mcp': 'MCP',
  'settings.layout.nav.skills': '技能',

  // Providers settings page
  'settings.providers.title': '提供方',
  'settings.providers.description': '設定本地 Koda 服務儲存的憑證與相容端點。',
  'settings.providers.addProvider': '新增提供方',

  // Sessions settings page
  'settings.sessions.title': '已封存的對話',
  'settings.sessions.description': '恢復希望重新回到活躍專案清單的對話。',
  'settings.sessions.empty.title': '沒有已封存的對話',
  'settings.sessions.empty.body': '已封存的對話會顯示在這裡。',
  'settings.sessions.restore': '恢復',

  // MCP settings page
  'settings.mcp.title': 'MCP 伺服器',
  'settings.mcp.description':
    '檢視從 ~/.koda/koda.yaml 載入的程序級 MCP 伺服器。重啟 Koda 以套用設定變更。',
  'settings.mcp.empty.title': '未設定 MCP 伺服器',
  'settings.mcp.empty.body':
    '在 ~/.koda/koda.yaml 中新增 mcp.servers 條目並重啟 Koda。',
  'settings.mcp.card.openAria': '開啟 {name}',
  'settings.mcp.card.toolCount.one': '{count} 個工具',
  'settings.mcp.card.toolCount.other': '{count} 個工具',
  'settings.mcp.card.mode.planAndBuild': '規劃 + 建構',
  'settings.mcp.card.mode.buildWithApproval': '建構(需核准)',
  'settings.mcp.details.id': 'ID',
  'settings.mcp.details.transport': '傳輸',
  'settings.mcp.details.agentModes': '代理模式',
  'settings.mcp.details.mode.planAndBuild': '規劃與建構',
  'settings.mcp.details.mode.buildWithApproval': '建構(需核准)',
  'settings.mcp.details.target': '目標',
  'settings.mcp.details.tools.title': '工具',
  'settings.mcp.details.tools.empty': '此伺服器啟動時未公開工具。',
  'settings.mcp.details.tools.mcpName': 'MCP 名稱:{name}',
  'settings.mcp.transport.http': 'HTTP',
  'settings.mcp.transport.stdio': 'stdio',
  'settings.mcp.transport.unknown': '未知',

  // Skills settings page
  'settings.skills.title': '技能',
  'settings.skills.description':
    '檢視本 Koda 程序啟動時從 ~/.koda/skills 載入的代理技能。重啟 Koda 以拾取檔案系統變更。',
  'settings.skills.empty.title': '未載入技能',
  'settings.skills.empty.body': '在 ~/.koda/skills 下新增技能並重啟 Koda。',
  'settings.skills.card.openAria': '開啟 {name}',
  'settings.skills.details.license': '授權',
  'settings.skills.details.compatibility': '相容性',
  'settings.skills.details.allowedTools': '允許的工具',
  'settings.skills.details.resources': '資源',
  'settings.skills.details.instructions': '指令',

  // App shell
  'app.shell.brand': 'Koda Studio',
  'app.shell.collapseSidebarAria': '收合側邊欄',
  'app.shell.newSession': '新增對話',
  'app.shell.new': '新增',
  'app.shell.projects': '專案',
  'app.shell.noSessions': '暫無對話',
  'app.shell.newSessionInAria': '在 {name} 中新增對話',
  'app.shell.newSessionInTitle': '在 {path} 中新增對話',
  'app.shell.settingsAria': '設定',

  // Home page
  'home.empty.title': '從一個對話開始',
  'home.empty.body': '從側邊欄建立對話。對話按本地專案分組。',
  'home.empty.configureProviders': '設定提供方',

  // Session page
  'session.notFound': '找不到對話',
  'session.empty.title': '準備就緒',
  'session.empty.body': '讓 Koda 檢查、規劃或修改此工作區。',
  'session.compaction.continuing': '沿用現有上下文繼續',
  'session.compaction.completed': '上下文已壓縮 · 第 {generation} 代',
  'session.compaction.failed': '上下文壓縮失敗',
  'session.compaction.inProgress': '正在壓縮較早的上下文…',
  'session.compaction.detail.completed':
    '{sourceTokens} 源 token · 預估壓縮後 {estimatedTokens}',
  'session.compaction.detail.inProgress': '當前上下文 {contextTokens} token',
  'session.compaction.boundary.label': '較早上下文已壓縮 · 第 {generation} 代',
  'session.compaction.boundary.titleGeneration': '第 {generation} 代',
  'session.compaction.boundary.titleEvents': '已摘要 {count} 個較早事件',
  'session.compaction.boundary.titleSourceTokens': '{count} 源 token',
  'session.compaction.boundary.titleEstimatedTokens':
    '壓縮後預估 {count} token',
  'session.compaction.boundary.titleModel': '模型:{modelId}',

  // Shared session labels
  'session.untitled': '未命名對話',

  // Theme toggle
  'theme.toggle.label': '主題',
  'theme.toggle.ariaLabel': '主題',
}
