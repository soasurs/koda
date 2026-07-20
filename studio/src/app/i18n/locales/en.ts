export const en = {
  // General settings
  'settings.general.title': 'General',
  'settings.general.description':
    'Manage appearance and conversation defaults for Koda Studio.',
  'settings.general.appearance.title': 'Appearance',
  'settings.general.appearance.theme': 'Theme',
  'settings.general.appearance.theme.system': 'System',
  'settings.general.appearance.theme.light': 'Light',
  'settings.general.appearance.theme.dark': 'Dark',
  'settings.general.appearance.language': 'Language',
  'settings.general.conversation.title': 'Conversation',
  'settings.general.conversation.expandReasoning':
    'Expand reasoning by default',
  'settings.general.conversation.expandReasoning.description':
    'Show the assistant reasoning panel expanded when a turn starts.',
  'settings.general.conversation.expandToolCalls':
    'Expand tool calls by default',
  'settings.general.conversation.expandToolCalls.description':
    'Show the tool call group expanded when tool calls run.',
  'settings.general.conversation.sendShortcut': 'Send shortcut',
  'settings.general.conversation.sendShortcut.enter': 'Enter',
  'settings.general.conversation.sendShortcut.shiftEnter': 'Shift + Enter',
  'settings.general.conversation.sendShortcut.commandEnter': 'Command + Enter',

  // Settings layout
  'settings.layout.title': 'Settings',
  'settings.layout.description':
    'Inspect process-wide capabilities and configure your local Koda service.',
  'settings.layout.nav.ariaLabel': 'Settings',
  'settings.layout.nav.providers': 'Providers',
  'settings.layout.nav.sessions': 'Sessions',
  'settings.layout.nav.mcp': 'MCP',
  'settings.layout.nav.skills': 'Skills',

  // Providers settings page
  'settings.providers.title': 'Providers',
  'settings.providers.description':
    'Configure credentials and compatible endpoints stored by your local Koda service.',
  'settings.providers.addProvider': 'Add provider',

  // Sessions settings page
  'settings.sessions.title': 'Archived sessions',
  'settings.sessions.description':
    'Restore sessions that you want to return to the active project list.',
  'settings.sessions.empty.title': 'No archived sessions',
  'settings.sessions.empty.body': 'Archived sessions will appear here.',
  'settings.sessions.restore': 'Restore',

  // MCP settings page
  'settings.mcp.title': 'MCP servers',
  'settings.mcp.description':
    'Inspect the process-wide MCP servers loaded from ~/.koda/koda.yaml. Restart Koda to apply configuration changes.',
  'settings.mcp.empty.title': 'No MCP servers configured',
  'settings.mcp.empty.body':
    'Add mcp.servers entries to ~/.koda/koda.yaml and restart Koda.',
  'settings.mcp.card.openAria': 'Open {name}',
  'settings.mcp.card.toolCount.one': '{count} tool',
  'settings.mcp.card.toolCount.other': '{count} tools',
  'settings.mcp.card.mode.planAndBuild': 'Plan + Build',
  'settings.mcp.card.mode.buildWithApproval': 'Build with approval',
  'settings.mcp.details.id': 'ID',
  'settings.mcp.details.transport': 'Transport',
  'settings.mcp.details.agentModes': 'Agent modes',
  'settings.mcp.details.mode.planAndBuild': 'Plan and Build',
  'settings.mcp.details.mode.buildWithApproval': 'Build with approval',
  'settings.mcp.details.target': 'Target',
  'settings.mcp.details.tools.title': 'Tools',
  'settings.mcp.details.tools.empty':
    'This server exposed no tools at startup.',
  'settings.mcp.details.tools.mcpName': 'MCP name: {name}',
  'settings.mcp.transport.http': 'HTTP',
  'settings.mcp.transport.stdio': 'stdio',
  'settings.mcp.transport.unknown': 'Unknown',

  // Skills settings page
  'settings.skills.title': 'Skills',
  'settings.skills.description':
    'Inspect the Agent Skills loaded from ~/.koda/skills when this Koda process started. Restart Koda to pick up filesystem changes.',
  'settings.skills.empty.title': 'No skills loaded',
  'settings.skills.empty.body':
    'Add a skill under ~/.koda/skills and restart Koda.',
  'settings.skills.card.openAria': 'Open {name}',
  'settings.skills.details.license': 'License',
  'settings.skills.details.compatibility': 'Compatibility',
  'settings.skills.details.allowedTools': 'Allowed tools',
  'settings.skills.details.resources': 'Resources',
  'settings.skills.details.instructions': 'Instructions',

  // App shell
  'app.shell.brand': 'Koda Studio',
  'app.shell.collapseSidebarAria': 'Collapse sidebar',
  'app.shell.newSession': 'New session',
  'app.shell.new': 'New',
  'app.shell.projects': 'Projects',
  'app.shell.noSessions': 'No sessions yet',
  'app.shell.newSessionInAria': 'New session in {name}',
  'app.shell.newSessionInTitle': 'New session in {path}',
  'app.shell.settingsAria': 'Settings',

  // Home page
  'home.empty.title': 'Start with a session',
  'home.empty.body':
    'Create a session from the sidebar. Sessions are organized by their local project.',
  'home.empty.configureProviders': 'Configure providers',

  // Session page
  'session.notFound': 'Session not found',
  'session.empty.title': 'Ready to work',
  'session.empty.body': 'Ask Koda to inspect, plan, or change this workspace.',
  'session.compaction.continuing': 'Continuing with existing context',
  'session.compaction.completed': 'Context compacted · generation {generation}',
  'session.compaction.failed': 'Context compaction failed',
  'session.compaction.inProgress': 'Compacting earlier context…',
  'session.compaction.detail.completed':
    '{sourceTokens} source tokens · {estimatedTokens} estimated after',
  'session.compaction.detail.inProgress':
    '{contextTokens} tokens in current context',
  'session.compaction.boundary.label':
    'Earlier context compacted · generation {generation}',
  'session.compaction.boundary.titleGeneration': 'Generation {generation}',
  'session.compaction.boundary.titleEvents':
    '{count} earlier events summarized',
  'session.compaction.boundary.titleSourceTokens': '{count} source tokens',
  'session.compaction.boundary.titleEstimatedTokens':
    '{count} estimated tokens after compaction',
  'session.compaction.boundary.titleModel': 'Model: {modelId}',

  // Shared session labels
  'session.untitled': 'Untitled session',

  // Theme toggle
  'theme.toggle.label': 'Theme',
  'theme.toggle.ariaLabel': 'Theme',
} satisfies Record<string, string>

export type EnDictionary = Record<keyof typeof en, string>
