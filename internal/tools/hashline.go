package tools

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const (
	anchorHashBytes   = 4
	revisionHashBytes = 12
)

type textFile struct {
	lines           []string
	trailingNewline bool
}

type anchoredLine struct {
	Number  int    `json:"number"`
	Anchor  string `json:"anchor"`
	Content string `json:"content"`
}

func parseTextFile(content string) textFile {
	if content == "" {
		return textFile{}
	}
	trailingNewline := strings.HasSuffix(content, "\n")
	if trailingNewline {
		content = strings.TrimSuffix(content, "\n")
	}
	return textFile{
		lines:           strings.Split(content, "\n"),
		trailingNewline: trailingNewline,
	}
}

func (f textFile) String() string {
	if len(f.lines) == 0 {
		return ""
	}
	content := strings.Join(f.lines, "\n")
	if f.trailingNewline {
		content += "\n"
	}
	return content
}

func (f textFile) revision() string {
	digest := sha256.Sum256([]byte(f.String()))
	return base64.RawURLEncoding.EncodeToString(digest[:revisionHashBytes])
}

func (f textFile) anchor(line int) (string, error) {
	if line < 1 || line > len(f.lines) {
		return "", fmt.Errorf("line %d is out of range", line)
	}
	previous, next := "", ""
	if line > 1 {
		previous = strings.TrimSuffix(f.lines[line-2], "\r")
	}
	if line < len(f.lines) {
		next = strings.TrimSuffix(f.lines[line], "\r")
	}
	current := strings.TrimSuffix(f.lines[line-1], "\r")
	digest := sha256.Sum256([]byte(previous + "\x00" + current + "\x00" + next))
	return strconv.Itoa(line) + ":" + base64.RawURLEncoding.EncodeToString(digest[:anchorHashBytes]), nil
}

func (f textFile) anchoredLines(start, end int, maxChars int) ([]anchoredLine, bool, int) {
	if start < 1 {
		start = 1
	}
	if end == 0 || end > len(f.lines) {
		end = len(f.lines)
	}
	if start > end || start > len(f.lines) {
		return nil, false, 0
	}
	if maxChars <= 0 {
		maxChars = defaultMaxChars
	}

	result := make([]anchoredLine, 0, end-start+1)
	used := 0
	for line := start; line <= end; line++ {
		anchor, err := f.anchor(line)
		if err != nil {
			return nil, false, 0
		}
		entry := anchoredLine{Number: line, Anchor: anchor, Content: f.lines[line-1]}
		entryChars := len([]rune(anchor)) + len([]rune(entry.Content)) + 3
		if len(result) > 0 && used+entryChars > maxChars {
			return result, true, line
		}
		if len(result) == 0 && entryChars > maxChars {
			entry.Content = truncateRunes(entry.Content, maxChars-len([]rune(anchor))-3)
			return []anchoredLine{entry}, true, line + 1
		}
		used += entryChars
		result = append(result, entry)
	}
	return result, false, 0
}

func parseAnchor(value string) (int, string, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, "", fmt.Errorf("invalid hashline anchor %q", value)
	}
	line, err := strconv.Atoi(parts[0])
	if err != nil || line < 1 {
		return 0, "", fmt.Errorf("invalid hashline anchor %q", value)
	}
	if _, err := base64.RawURLEncoding.DecodeString(parts[1]); err != nil {
		return 0, "", fmt.Errorf("invalid hashline anchor %q", value)
	}
	return line, parts[1], nil
}

func (f textFile) verifyAnchor(value string) (int, error) {
	line, hash, err := parseAnchor(value)
	if err != nil {
		return 0, err
	}
	actual, err := f.anchor(line)
	if err != nil {
		return 0, err
	}
	_, actualHash, _ := strings.Cut(actual, ":")
	if hash != actualHash {
		return 0, fmt.Errorf("stale anchor %q; current anchor is %q", value, actual)
	}
	return line, nil
}

func splitEditContent(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSuffix(content, "\n")
	return strings.Split(content, "\n")
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}
