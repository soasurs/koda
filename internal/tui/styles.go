package tui

import "github.com/charmbracelet/lipgloss"

// foldThreshold is the number of lines above which a tool result is collapsed by default.
const foldThreshold = 10

var (
	// ── user bubble ──────────────────────────────────────────────────────────
	userBubbleStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("111")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 2).
			MarginLeft(2)

	// ── assistant bubble ─────────────────────────────────────────────────────
	assistantBubbleStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderForeground(lipgloss.Color("180")).
				Foreground(lipgloss.Color("252")).
				Padding(0, 2).
				MarginLeft(2)

	// ── thinking bubble ──────────────────────────────────────────────────────
	thinkingBubbleStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderForeground(lipgloss.Color("105")).
				Foreground(lipgloss.Color("245")).
				Italic(true).
				Padding(0, 2).
				MarginLeft(2)

	thinkingLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("105")).
				Bold(true)

	systemBubbleStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderForeground(lipgloss.Color("240")).
				Foreground(lipgloss.Color("246")).
				Padding(0, 2).
				MarginLeft(2)

	// ── tool call ─────────────────────────────────────────────────────────────
	toolCallCardStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderForeground(lipgloss.Color("214")).
				Foreground(lipgloss.Color("250")).
				Padding(0, 2).
				MarginLeft(4)

	toolCallCardFocusedStyle = toolCallCardStyle.
					BorderForeground(lipgloss.Color("221"))

	toolCallPrefixStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true)

	toolCallArgsStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("222"))

	// ── tool result ───────────────────────────────────────────────────────────
	toolResultMetaStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244"))

	toolResultCardStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 2).
				MarginLeft(4).
				Foreground(lipgloss.Color("250"))

	toolResultCardFocusedStyle = toolResultCardStyle.
					BorderForeground(lipgloss.Color("109"))

	// ── focus indicator (shown next to focused foldable message) ─────────────
	focusIndicatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("62")).
				Bold(true)

	// ── status bar ────────────────────────────────────────────────────────────
	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	statusModelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("153"))

	statusProviderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("180")).
				Bold(true)

	statusHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))

	// ── input area ────────────────────────────────────────────────────────────
	inputSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("237"))

	// ── slash menu ────────────────────────────────────────────────────────────
	slashMenuBoxStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)

	slashMenuTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("186")).
				Bold(true)

	slashMenuItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	slashMenuSelectedStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderForeground(lipgloss.Color("109")).
				Foreground(lipgloss.Color("255")).
				Bold(true)

	slashMenuMetaStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244"))

	// ── chooser modal ─────────────────────────────────────────────────────────
	modalBackdropStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("0")).
				Foreground(lipgloss.Color("252"))

	modalBoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("109")).
			Background(lipgloss.Color("0")).
			Padding(0, 2)

	modalTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("186")).
			Bold(true)

	modalBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	modalHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	modalOptionStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("0")).
				Foreground(lipgloss.Color("250")).
				PaddingLeft(1)

	modalOptionSelectedStyle = lipgloss.NewStyle().
					BorderStyle(lipgloss.NormalBorder()).
					BorderLeft(true).
					BorderForeground(lipgloss.Color("109")).
					Foreground(lipgloss.Color("255")).
					Bold(true).
					PaddingLeft(1)

	modalWindowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("0"))

	modalInputStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("0")).
			Foreground(lipgloss.Color("255")).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	// ── empty state ───────────────────────────────────────────────────────────
	emptyStateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)
)
