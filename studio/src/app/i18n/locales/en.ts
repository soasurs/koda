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
  'session.header.untitled': 'Untitled',

  // Reasoning view
  'session.reasoning.thinking': 'Thinking',
  'session.reasoning.thought': 'Thought',

  // Tool activity
  'session.tool.tools': 'Tools',
  'session.tool.steps.one': '{count} tool step',
  'session.tool.steps.other': '{count} tool steps',
  'session.tool.status.completed': 'Completed',
  'session.tool.status.failed': 'Failed',
  'session.tool.status.running': 'Running...',
  'session.tool.output.title': 'Output',
  'session.tool.output.exit': 'Exit {code}',
  'session.tool.output.truncated': ' · Truncated',
  'session.tool.output.empty': 'No output',
  'session.tool.diff.truncated': 'Truncated',

  // Tool names (present tense)
  'tools.name.ask_questions': 'Ask questions',
  'tools.name.create_file': 'Create file',
  'tools.name.edit_file': 'Edit file',
  'tools.name.find_files': 'Find files',
  'tools.name.list_directory': 'List directory',
  'tools.name.read_file': 'Read file',
  'tools.name.run_shell': 'Run command',
  'tools.name.search_text': 'Search text',
  'tools.name.web_fetch': 'Web fetch',
  'tools.name.write_file': 'Write file',
  'tools.name.load_skill': 'Load skill',
  // Tool names (past tense)
  'tools.namePast.ask_questions': 'Asked questions',
  'tools.namePast.create_file': 'Created file',
  'tools.namePast.edit_file': 'Edited file',
  'tools.namePast.find_files': 'Found files',
  'tools.namePast.list_directory': 'Listed directory',
  'tools.namePast.read_file': 'Read file',
  'tools.namePast.run_shell': 'Ran command',
  'tools.namePast.search_text': 'Searched text',
  'tools.namePast.web_fetch': 'Fetched web page',
  'tools.namePast.write_file': 'Wrote file',
  'tools.namePast.load_skill': 'Loaded skill',

  // Session turn
  'session.turn.failed': 'Turn failed',
  'session.turn.interrupted': 'Turn interrupted',
  'session.turn.earlierActivity': 'Earlier activity',
  'session.turn.editMessage': 'Edit message',
  'session.turn.retryTurn': 'Retry turn',
  'session.turn.copyResponse': 'Copy response',
  'session.turn.copied': 'Copied!',
  'session.turn.cancelEditing': 'Cancel editing',
  'session.turn.send': 'Send',
  'session.turn.interruption.canceled': 'Canceled by the user',
  'session.turn.interruption.deadline': 'Execution timed out',
  'session.turn.interruption.consumer': 'The client stopped receiving the turn',
  'session.turn.interruption.abandoned':
    'Recovered after an earlier Koda process stopped',
  'session.turn.interruption.default': 'Execution stopped before completion',
  'session.turn.failure.location.agent': 'agent',
  'session.turn.failure.location.provider': 'provider',
  'session.turn.failure.location.tool': 'tool',
  'session.turn.failure.location.storage': 'storage',
  'session.turn.failure.location.client': 'client',
  'session.turn.failure.inLocation': 'Execution failed in the {location}',
  'session.turn.failure.generic': 'Execution failed',
  'session.turn.shortcut.shiftEnter': 'Shift + Enter',
  'session.turn.shortcut.commandEnter': '⌘ + Enter',
  'session.turn.shortcut.enter': 'Enter',

  // Session composer
  'session.composer.message': 'Message',
  'session.composer.stop': 'Stop',
  'session.composer.send': 'Send',
  'session.composer.chooseSendShortcut': 'Choose send shortcut',
  'session.composer.sendWith': 'Send message with',
  'session.composer.placeholder': 'Ask Koda to work in this directory…',
  'session.composer.disclaimer':
    'Koda can make mistakes. Review commands and file changes.',
  'session.composer.mode.build': 'Build',
  'session.composer.mode.plan': 'Plan',

  // Session model picker
  'session.modelPicker.settingsAria': 'Session model settings',
  'session.modelPicker.provider': 'Provider',
  'session.modelPicker.model': 'Model',
  'session.modelPicker.reasoningEffort': 'Reasoning effort',
  'session.modelPicker.providerDefault': 'Provider default',
  'session.modelPicker.sessionProvider': 'Session provider',
  'session.modelPicker.sessionModel': 'Session model',
  'session.modelPicker.sessionReasoningEffort': 'Session reasoning effort',
  'session.modelPicker.cancel': 'Cancel',
  'session.modelPicker.apply': 'Apply',
  'session.modelPicker.default': 'default',

  // Session header
  'session.header.contextUsageUnavailable':
    'Context usage unavailable · {window} window',
  'session.header.contextShort': 'Context — / {window}',
  'session.header.usageLabel':
    '{used} tokens used, {remaining} remaining, {percentage}% of {window}',
  'session.header.usageSummary':
    '{used} used · {remaining} left · {percentage}% of {window}',

  // Session list item
  'session.listItem.rename': 'Rename',
  'session.listItem.archive': 'Archive',
  'session.listItem.actionsAria': 'Actions for {label}',

  // Directory picker
  'directory.picker.description':
    'Browse directories on the machine running Koda.',
  'directory.picker.title': 'Choose workspace',
  'directory.picker.home': 'Home',
  'directory.picker.parent': 'Parent directory',
  'directory.picker.noChildren': 'No child directories',
  'directory.picker.cancel': 'Cancel',
  'directory.picker.select': 'Select this directory',

  // Create session dialog
  'createSession.description':
    'Select a workspace and model for the new coding session.',
  'createSession.title': 'New session',
  'createSession.workspace': 'Workspace',
  'createSession.chooseDirectory': 'Choose a local directory',
  'createSession.provider': 'Provider',
  'createSession.noProviders': 'No configured providers',
  'createSession.model': 'Model',
  'createSession.noModels': 'No models available',
  'createSession.reasoningEffort': 'Reasoning effort',
  'createSession.providerDefault': 'Provider default',
  'createSession.fileAccess': 'File access',
  'createSession.fileAccess.workspaceRead': 'Workspace read',
  'createSession.fileAccess.workspaceWrite': 'Workspace write',
  'createSession.fileAccess.unrestricted': 'Unrestricted',
  'createSession.shellAccess': 'Shell access',
  'createSession.shellAccess.askEveryTime': 'Ask every time',
  'createSession.shellAccess.unrestricted': 'Unrestricted',
  'createSession.cancel': 'Cancel',
  'createSession.submit': 'Create session',

  // Rename session dialog
  'renameSession.title': 'Rename session',
  'renameSession.description':
    'Choose a name that makes this session easy to find.',
  'renameSession.name': 'Name',
  'renameSession.placeholder': 'Session name',
  'renameSession.cancel': 'Cancel',
  'renameSession.submit': 'Rename',

  // Run prompts
  'runPrompt.permissionRequired': 'Permission required',
  'runPrompt.permissionBody': 'Koda wants to perform the following action.',
  'runPrompt.reviewChanges': 'Review proposed changes',
  'runPrompt.approve': 'Approve',
  'runPrompt.reject': 'Reject',
  'runPrompt.submitAnswers': 'Submit answers',
  'runPrompt.cancel': 'Cancel',
  'runPrompt.freeformPlaceholder': 'Or write your own answer',

  // Provider card
  'provider.card.builtIn': 'Built in',
  'provider.card.loadingModels': 'Loading models…',
  'provider.card.modelCount': '{count} models',
  'provider.card.ready': 'Ready',
  'provider.card.notConfigured': 'Not configured',
  'provider.card.refreshAria': 'Refresh {name} models',
  'provider.card.refreshTitle': 'Refresh models',
  'provider.card.configure': 'Configure',
  'provider.card.deleteAria': 'Delete {name}',
  'provider.card.deleteConfirm': 'Delete {name}?',
  'provider.card.models': 'Models',
  'provider.card.addModel': 'Add model',
  'provider.card.noModels': 'No models available',
  'provider.card.deleteModelPrompt': 'Delete this model?',
  'provider.card.confirmDeleteAria': 'Confirm delete',
  'provider.card.cancelDeleteAria': 'Cancel delete',
  'provider.card.reasoning': 'Reasoning: {efforts}',
  'provider.card.context': 'Context: {tokens}',
  'provider.card.editModelAria': 'Edit {id}',
  'provider.card.deleteModelAria': 'Delete {id}',
  'provider.card.enableGeneration': 'Enable this provider for agent generation',

  // Provider dialog
  'provider.dialog.editDescription':
    'Leave the API key empty to keep the existing credential.',
  'provider.dialog.addDescription':
    'Add a provider or an API-compatible endpoint.',
  'provider.dialog.editTitle': 'Configure {name}',
  'provider.dialog.addTitle': 'Add provider',
  'provider.dialog.id': 'ID',
  'provider.dialog.displayName': 'Display name',
  'provider.dialog.apiType': 'API type',
  'provider.dialog.selectApi': 'Select an API',
  'provider.dialog.baseUrl': 'Base URL',
  'provider.dialog.baseUrlPlaceholder': 'Use provider default',
  'provider.dialog.apiKey': 'API key',
  'provider.dialog.apiKeyKeep': 'Keep existing key',
  'provider.dialog.apiKeyRequired': 'Required',
  'provider.dialog.cancel': 'Cancel',
  'provider.dialog.save': 'Save provider',

  // Add model dialog
  'addModel.description':
    'The model ID is sent to the provider API exactly as entered.',
  'addModel.title': 'Add model to {name}',
  'addModel.modelId': 'Model ID',
  'addModel.displayName': 'Display name',
  'addModel.displayNamePlaceholder': 'Defaults to model ID',
  'addModel.contextWindow': 'Context window tokens',
  'addModel.contextPlaceholder': 'Uses catalog or Koda fallback',
  'addModel.contextHelp':
    "Optional total input and output capacity. Leave empty to use catalog metadata or Koda's fallback.",
  'addModel.duplicate': 'A model with this ID already exists.',
  'addModel.contextInvalid': 'Context window must be a positive integer.',
  'addModel.cancel': 'Cancel',
  'addModel.submit': 'Add model',

  // Edit model dialog
  'editModel.description':
    'Edit model metadata. Changes take effect immediately.',
  'editModel.title': 'Edit {id}',
  'editModel.modelId': 'Model ID',
  'editModel.displayName': 'Display name',
  'editModel.displayNamePlaceholder': 'Defaults to model ID',
  'editModel.reasoningEfforts': 'Reasoning efforts',
  'editModel.reasoningPlaceholder': 'e.g. low, medium, high, max',
  'editModel.reasoningHelp':
    'Comma-separated; leave empty to disable reasoning.',
  'editModel.contextWindow': 'Context window tokens',
  'editModel.contextPlaceholder': 'Uses catalog or Koda fallback',
  'editModel.contextHelp':
    "Optional total input and output capacity. Leave empty to use catalog metadata or Koda's fallback.",
  'editModel.defaultReasoningEffort': 'Default reasoning effort',
  'editModel.providerDefault': 'Provider default',
  'editModel.contextInvalid': 'Context window must be a positive integer.',
  'editModel.cancel': 'Cancel',
  'editModel.save': 'Save',

  // Theme toggle
  'theme.toggle.label': 'Theme',
  'theme.toggle.ariaLabel': 'Theme',
} satisfies Record<string, string>

export type EnDictionary = Record<keyof typeof en, string>
