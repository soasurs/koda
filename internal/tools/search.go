package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/permission"
)

type searchTextInput struct {
	Pattern       string   `json:"pattern" jsonschema:"Ripgrep regular expression to find"`
	Path          string   `json:"path,omitempty" jsonschema:"File or directory to search; defaults to the session workspace"`
	Globs         []string `json:"globs,omitempty" jsonschema:"Optional ripgrep glob filters such as **/*.go"`
	FixedStrings  bool     `json:"fixed_strings,omitempty" jsonschema:"Treat pattern as literal text instead of a regular expression"`
	CaseSensitive bool     `json:"case_sensitive,omitempty" jsonschema:"Use case-sensitive matching; the default is smart case"`
	IncludeHidden bool     `json:"include_hidden,omitempty" jsonschema:"Include hidden files, excluding .git"`
	MaxResults    int      `json:"max_results,omitempty" jsonschema:"Maximum matches to return; defaults to 200 and is capped"`
	MaxChars      int      `json:"max_chars,omitempty" jsonschema:"Maximum returned characters; defaults to 32768 and is capped"`
}

type searchMatch struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Anchor   string `json:"anchor"`
	Content  string `json:"content"`
	Revision string `json:"revision"`
}

type searchTextOutput struct {
	Matches   []searchMatch `json:"matches"`
	Truncated bool          `json:"truncated"`
}

type findFilesInput struct {
	Path          string   `json:"path,omitempty" jsonschema:"Directory to search; defaults to the session workspace"`
	Globs         []string `json:"globs,omitempty" jsonschema:"Optional ripgrep glob filters such as **/*_test.go"`
	IncludeHidden bool     `json:"include_hidden,omitempty" jsonschema:"Include hidden files, excluding .git"`
	MaxResults    int      `json:"max_results,omitempty" jsonschema:"Maximum files to return; defaults to 200 and is capped"`
	MaxChars      int      `json:"max_chars,omitempty" jsonschema:"Maximum returned characters; defaults to 32768 and is capped"`
}

type findFilesOutput struct {
	Files     []string `json:"files"`
	Truncated bool     `json:"truncated"`
}

type rgMessage struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
	} `json:"data"`
}

func (s service) newSearchTextTool() (tool.Tool, error) {
	return tool.NewFunc(tool.Definition{
		Name:        "search_text",
		Description: "Search UTF-8 file contents with ripgrep. Results include hashline anchors and file revisions usable by edit_file.",
	}, s.searchText)
}

func (s service) searchText(ctx context.Context, input searchTextInput) (searchTextOutput, error) {
	if strings.TrimSpace(input.Pattern) == "" {
		return searchTextOutput{}, handled(errors.New("pattern must not be empty"))
	}
	if strings.TrimSpace(input.Path) == "" {
		input.Path = "."
	}
	root, err := s.resolver.existing(input.Path)
	if err != nil {
		return searchTextOutput{}, handled(err)
	}
	if err := s.authorize(ctx, permission.KindFileRead, root.scope, absoluteTargets(root), "search "+root.display, nil); err != nil {
		return searchTextOutput{}, err
	}

	args := []string{"--no-config", "--json", "--line-number", "--no-messages"}
	if input.FixedStrings {
		args = append(args, "--fixed-strings")
	}
	if input.CaseSensitive {
		args = append(args, "--case-sensitive")
	} else {
		args = append(args, "--smart-case")
	}
	if input.IncludeHidden {
		args = append(args, "--hidden", "--glob", "!.git")
	}
	for _, glob := range input.Globs {
		glob = strings.TrimSpace(glob)
		if glob != "" {
			args = append(args, "--glob", glob)
		}
	}
	args = append(args, "--", input.Pattern, root.real)

	commandCtx, cancel := context.WithTimeout(ctx, s.commandTimeout)
	defer cancel()
	command := exec.CommandContext(commandCtx, "rg", args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return searchTextOutput{}, handled(fmt.Errorf("start rg: %w", err))
	}
	if err := command.Start(); err != nil {
		return searchTextOutput{}, handled(fmt.Errorf("start rg: %w", err))
	}

	limit := clamp(input.MaxResults, defaultMaxResults, defaultMaxResults)
	maxChars := clamp(input.MaxChars, defaultMaxChars, defaultMaxChars)
	decoder := json.NewDecoder(stdout)
	cache := make(map[string]textFile)
	output := searchTextOutput{Matches: make([]searchMatch, 0, limit)}
	used := 0
	for {
		var message rgMessage
		err := decoder.Decode(&message)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			cancel()
			_ = command.Wait()
			return searchTextOutput{}, handled(fmt.Errorf("decode rg output: %w", err))
		}
		if message.Type != "match" {
			continue
		}
		path, err := searchMatchPath(root, message.Data.Path.Text)
		if err != nil {
			cancel()
			_ = command.Wait()
			return searchTextOutput{}, handled(err)
		}
		file, ok := cache[path.real]
		if !ok {
			file, err = loadTextFile(path.real)
			if err != nil {
				cancel()
				_ = command.Wait()
				return searchTextOutput{}, handled(err)
			}
			cache[path.real] = file
		}
		if message.Data.LineNumber < 1 || message.Data.LineNumber > len(file.lines) {
			cancel()
			_ = command.Wait()
			return searchTextOutput{}, handled(errors.New("rg returned an invalid line number"))
		}
		content := file.lines[message.Data.LineNumber-1]
		rgContent := strings.TrimSuffix(message.Data.Lines.Text, "\n")
		if content != rgContent {
			continue
		}
		anchor, err := file.anchor(message.Data.LineNumber)
		if err != nil {
			continue
		}
		entryChars := len([]rune(path.display)) + len([]rune(anchor)) + len([]rune(content)) + 16
		if len(output.Matches) == limit || (len(output.Matches) > 0 && used+entryChars > maxChars) {
			output.Truncated = true
			cancel()
			break
		}
		if len(output.Matches) == 0 && entryChars > maxChars {
			content = truncateRunes(content, maxChars-len([]rune(path.display))-len([]rune(anchor))-16)
			output.Truncated = true
		}
		used += entryChars
		output.Matches = append(output.Matches, searchMatch{
			Path:     path.display,
			Line:     message.Data.LineNumber,
			Anchor:   anchor,
			Content:  content,
			Revision: file.revision(),
		})
		if output.Truncated {
			cancel()
			break
		}
	}
	if err := command.Wait(); err != nil && !output.Truncated {
		if ctx.Err() != nil {
			return searchTextOutput{}, ctx.Err()
		}
		if commandCtx.Err() != nil {
			return searchTextOutput{}, handled(errors.New("rg search timed out"))
		}
		// rg exits with status 1 when no line matches. It is not an error.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return searchTextOutput{}, handled(fmt.Errorf("run rg: %w", err))
		}
	}
	return output, nil
}

func (s service) newFindFilesTool() (tool.Tool, error) {
	return tool.NewFunc(tool.Definition{
		Name:        "find_files",
		Description: "Find paths with ripgrep --files. Results respect ignore files unless hidden files are explicitly requested.",
	}, s.findFiles)
}

func (s service) findFiles(ctx context.Context, input findFilesInput) (findFilesOutput, error) {
	if strings.TrimSpace(input.Path) == "" {
		input.Path = "."
	}
	root, err := s.resolver.existing(input.Path)
	if err != nil {
		return findFilesOutput{}, handled(err)
	}
	if !root.info.IsDir() {
		return findFilesOutput{}, handled(errors.New("path is not a directory"))
	}
	if err := s.authorize(ctx, permission.KindFileRead, root.scope, absoluteTargets(root), "find files in "+root.display, nil); err != nil {
		return findFilesOutput{}, err
	}

	args := []string{"--no-config", "--files", "--no-messages"}
	if input.IncludeHidden {
		args = append(args, "--hidden", "--glob", "!.git")
	}
	for _, glob := range input.Globs {
		glob = strings.TrimSpace(glob)
		if glob != "" {
			args = append(args, "--glob", glob)
		}
	}
	args = append(args, root.real)
	commandCtx, cancel := context.WithTimeout(ctx, s.commandTimeout)
	defer cancel()
	command := exec.CommandContext(commandCtx, "rg", args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return findFilesOutput{}, handled(fmt.Errorf("start rg: %w", err))
	}
	if err := command.Start(); err != nil {
		return findFilesOutput{}, handled(fmt.Errorf("start rg: %w", err))
	}

	limit := clamp(input.MaxResults, defaultMaxResults, defaultMaxResults)
	maxChars := clamp(input.MaxChars, defaultMaxChars, defaultMaxChars)
	output := findFilesOutput{Files: make([]string, 0, limit)}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024), maxTextFileBytes)
	used := 0
	for scanner.Scan() {
		path, err := searchMatchPath(root, scanner.Text())
		if err != nil {
			cancel()
			_ = command.Wait()
			return findFilesOutput{}, handled(err)
		}
		entryChars := len([]rune(path.display)) + 1
		if len(output.Files) == limit || (len(output.Files) > 0 && used+entryChars > maxChars) {
			output.Truncated = true
			cancel()
			break
		}
		output.Files = append(output.Files, path.display)
		used += entryChars
	}
	if err := scanner.Err(); err != nil && !output.Truncated {
		cancel()
		_ = command.Wait()
		return findFilesOutput{}, handled(fmt.Errorf("read rg output: %w", err))
	}
	if err := command.Wait(); err != nil && !output.Truncated {
		if ctx.Err() != nil {
			return findFilesOutput{}, ctx.Err()
		}
		if commandCtx.Err() != nil {
			return findFilesOutput{}, handled(errors.New("file search timed out"))
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return findFilesOutput{}, handled(fmt.Errorf("run rg: %w", err))
		}
	}
	return output, nil
}

func searchMatchPath(root resolvedPath, value string) (resolvedPath, error) {
	if !filepath.IsAbs(value) {
		value = filepath.Join(root.real, value)
	}
	resolver := resolver{workspace: root.workspace}
	return resolver.existing(value)
}
