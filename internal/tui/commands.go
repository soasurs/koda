package tui

import "strings"

type slashMode int

const (
	slashModeNone slashMode = iota
	slashModeRoot
)

type slashOption struct {
	Title       string
	Value       string
	Description string
}

type slashState struct {
	Mode     slashMode
	Query    string
	Options  []slashOption
	Selected int
}

func (s slashState) Visible() bool {
	return s.Mode != slashModeNone && len(s.Options) > 0
}

func parseSlashInput(input string) (slashMode, string) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return slashModeNone, ""
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return slashModeRoot, ""
	}

	cmd := fields[0]
	arg := ""
	if len(fields) > 1 {
		arg = strings.Join(fields[1:], " ")
	}

	switch cmd {
	case "/model":
		return slashModeNone, arg
	case "/connect":
		return slashModeNone, arg
	case "/sessions":
		return slashModeNone, arg
	case "/exit":
		return slashModeNone, arg
	case "/undo":
		return slashModeNone, arg
	default:
		return slashModeRoot, strings.TrimPrefix(trimmed, "/")
	}
}

func filterSlashOptions(options []slashOption, query string) []slashOption {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return options
	}

	filtered := make([]slashOption, 0, len(options))
	for _, option := range options {
		if strings.Contains(strings.ToLower(option.Title), query) ||
			strings.Contains(strings.ToLower(option.Value), query) ||
			strings.Contains(strings.ToLower(option.Description), query) {
			filtered = append(filtered, option)
		}
	}
	return filtered
}
