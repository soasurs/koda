package tools

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/soasurs/adk/tool"
	"github.com/soasurs/koda/internal/permission"
)

const maxTextFileBytes = 4 << 20

type readFileInput struct {
	Path      string `json:"path" jsonschema:"Path to read, relative to the session workspace unless absolute"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"First 1-based line to return; defaults to 1"`
	EndLine   int    `json:"end_line,omitempty" jsonschema:"Last 1-based line to return; defaults to the end of the file"`
	MaxChars  int    `json:"max_chars,omitempty" jsonschema:"Maximum returned characters; defaults to 32768 and is capped"`
}

type readFileOutput struct {
	Path       string         `json:"path"`
	Revision   string         `json:"revision"`
	Lines      []anchoredLine `json:"lines"`
	Truncated  bool           `json:"truncated"`
	NextLine   int            `json:"next_line,omitempty"`
	TotalLines int            `json:"total_lines"`
}

type listDirectoryInput struct {
	Path          string `json:"path,omitempty" jsonschema:"Directory to list; defaults to the session workspace"`
	IncludeHidden bool   `json:"include_hidden,omitempty" jsonschema:"Include entries whose names begin with a dot"`
	MaxEntries    int    `json:"max_entries,omitempty" jsonschema:"Maximum entries to return; defaults to 200 and is capped"`
}

type directoryEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Symlink   bool   `json:"symlink"`
	Size      int64  `json:"size,omitempty"`
}

type listDirectoryOutput struct {
	Path      string           `json:"path"`
	Entries   []directoryEntry `json:"entries"`
	Truncated bool             `json:"truncated"`
}

type writeFileInput struct {
	Path             string `json:"path" jsonschema:"Path to overwrite, relative to the session workspace unless absolute"`
	Content          string `json:"content" jsonschema:"Complete file content to write"`
	ExpectedRevision string `json:"expected_revision,omitempty" jsonschema:"Optional revision returned by read_file; rejects a stale whole-file write"`
}

type createFileInput struct {
	Path    string `json:"path" jsonschema:"Path for a new file, relative to the session workspace unless absolute"`
	Content string `json:"content" jsonschema:"Initial file content"`
}

type fileWriteOutput struct {
	Path        string       `json:"path"`
	Bytes       int          `json:"bytes"`
	Lines       int          `json:"lines"`
	Revision    string       `json:"revision"`
	FileChanges []FileChange `json:"file_changes"`
}

type editFileInput struct {
	Path             string          `json:"path" jsonschema:"Existing file to modify, relative to the session workspace unless absolute"`
	ExpectedRevision string          `json:"expected_revision" jsonschema:"Revision returned by read_file or search_text; rejects any stale edit"`
	Edits            []editOperation `json:"edits" jsonschema:"One or more non-overlapping hashline edits"`
}

type editOperation struct {
	Operation string `json:"operation" jsonschema:"replace, delete, insert_before, or insert_after"`
	Start     string `json:"start,omitempty" jsonschema:"Start hashline anchor for replace or delete"`
	End       string `json:"end,omitempty" jsonschema:"End hashline anchor for replace or delete; defaults to start"`
	Anchor    string `json:"anchor,omitempty" jsonschema:"Hashline anchor for insert_before or insert_after"`
	Content   string `json:"content,omitempty" jsonschema:"Replacement or inserted content"`
}

type editFileOutput struct {
	Path        string         `json:"path"`
	Revision    string         `json:"revision"`
	Anchors     []anchoredLine `json:"anchors"`
	FileChanges []FileChange   `json:"file_changes"`
}

func (s service) newReadFileTool() (tool.Tool, error) {
	return tool.NewFunc(tool.Definition{
		Name:        "read_file",
		Description: "Read a UTF-8 text file as hash-anchored lines. Use the returned revision and anchors with edit_file.",
	}, s.readFile)
}

func (s service) readFile(ctx context.Context, input readFileInput) (readFileOutput, error) {
	path, err := s.resolver.existing(input.Path)
	if err != nil {
		return readFileOutput{}, handled(err)
	}
	if path.info.IsDir() {
		return readFileOutput{}, handled(errors.New("path is a directory"))
	}
	if err := s.authorize(ctx, permission.KindFileRead, path.scope, absoluteTargets(path), "read "+path.display, nil); err != nil {
		return readFileOutput{}, err
	}
	file, err := loadTextFile(path.real)
	if err != nil {
		return readFileOutput{}, handled(err)
	}
	if input.StartLine < 0 || input.EndLine < 0 {
		return readFileOutput{}, handled(errors.New("line numbers must not be negative"))
	}
	maxChars := clamp(input.MaxChars, defaultMaxChars, defaultMaxChars)
	lines, truncated, nextLine := file.anchoredLines(input.StartLine, input.EndLine, maxChars)
	return readFileOutput{
		Path:       path.display,
		Revision:   file.revision(),
		Lines:      lines,
		Truncated:  truncated,
		NextLine:   nextLine,
		TotalLines: len(file.lines),
	}, nil
}

func (s service) newListDirectoryTool() (tool.Tool, error) {
	return tool.NewFunc(tool.Definition{
		Name:        "list_directory",
		Description: "List one directory level. Use find_files for recursive filename search.",
	}, s.listDirectory)
}

func (s service) listDirectory(ctx context.Context, input listDirectoryInput) (listDirectoryOutput, error) {
	if strings.TrimSpace(input.Path) == "" {
		input.Path = "."
	}
	path, err := s.resolver.existing(input.Path)
	if err != nil {
		return listDirectoryOutput{}, handled(err)
	}
	if !path.info.IsDir() {
		return listDirectoryOutput{}, handled(errors.New("path is not a directory"))
	}
	if err := s.authorize(ctx, permission.KindFileRead, path.scope, absoluteTargets(path), "list "+path.display, nil); err != nil {
		return listDirectoryOutput{}, err
	}
	entries, err := os.ReadDir(path.real)
	if err != nil {
		return listDirectoryOutput{}, handled(fmt.Errorf("read directory: %w", err))
	}
	limit := clamp(input.MaxEntries, defaultMaxEntries, defaultMaxEntries)
	result := make([]directoryEntry, 0, min(limit, len(entries)))
	truncated := false
	for _, entry := range entries {
		if !input.IncludeHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if len(result) == limit {
			truncated = true
			break
		}
		info, infoErr := entry.Info()
		item := directoryEntry{
			Name:      entry.Name(),
			Path:      filepath.Join(path.display, entry.Name()),
			Directory: entry.IsDir(),
			Symlink:   entry.Type()&os.ModeSymlink != 0,
		}
		if infoErr == nil && !item.Directory {
			item.Size = info.Size()
		}
		result = append(result, item)
	}
	return listDirectoryOutput{Path: path.display, Entries: result, Truncated: truncated}, nil
}

func (s service) newWriteFileTool() (tool.Tool, error) {
	return tool.NewFunc(tool.Definition{
		Name:        "write_file",
		Description: "Overwrite a complete UTF-8 file. It may create missing parent directories and returns a display diff.",
	}, s.writeFile)
}

func (s service) writeFile(ctx context.Context, input writeFileInput) (fileWriteOutput, error) {
	path, err := s.resolver.writeTarget(input.Path)
	if err != nil {
		return fileWriteOutput{}, handled(err)
	}
	before, err := loadTextFileIfExists(path.real)
	if err != nil {
		return fileWriteOutput{}, handled(err)
	}
	if input.ExpectedRevision != "" && input.ExpectedRevision != before.revision() {
		return fileWriteOutput{}, handled(errors.New("stale file revision; read the file again before overwriting it"))
	}
	after := parseTextFile(input.Content)
	change := wholeFileChange(path.display, before, after, fileChangeKind(path.exists))
	if err := s.authorize(ctx, permission.KindFileWrite, path.scope, absoluteTargets(path), "write "+path.display, []FileChange{change}); err != nil {
		return fileWriteOutput{}, err
	}
	// Re-plan after a possibly long approval wait so the returned diff is the
	// actual mutation. An explicit expected revision remains a strict guard.
	approvedPath := path
	path, err = s.resolver.writeTarget(input.Path)
	if err != nil {
		return fileWriteOutput{}, handled(err)
	}
	before, err = loadTextFileIfExists(path.real)
	if err != nil {
		return fileWriteOutput{}, handled(err)
	}
	if input.ExpectedRevision != "" && input.ExpectedRevision != before.revision() {
		return fileWriteOutput{}, handled(errors.New("stale file revision; read the file again before overwriting it"))
	}
	change = wholeFileChange(path.display, before, after, fileChangeKind(path.exists))
	if !sameTarget(approvedPath, path) {
		if err := s.authorize(ctx, permission.KindFileWrite, path.scope, absoluteTargets(path), "write "+path.display, []FileChange{change}); err != nil {
			return fileWriteOutput{}, err
		}
	}
	if err := writeAtomic(path.real, input.Content, path.info); err != nil {
		return fileWriteOutput{}, handled(err)
	}
	return fileWriteOutput{
		Path:        path.display,
		Bytes:       len(input.Content),
		Lines:       len(after.lines),
		Revision:    after.revision(),
		FileChanges: []FileChange{change},
	}, nil
}

func (s service) newCreateFileTool() (tool.Tool, error) {
	return tool.NewFunc(tool.Definition{
		Name:        "create_file",
		Description: "Create a new UTF-8 file and fail if it already exists. Returns a display diff.",
	}, s.createFile)
}

func (s service) createFile(ctx context.Context, input createFileInput) (fileWriteOutput, error) {
	path, err := s.resolver.writeTarget(input.Path)
	if err != nil {
		return fileWriteOutput{}, handled(err)
	}
	if path.exists {
		return fileWriteOutput{}, handled(errors.New("file already exists"))
	}
	after := parseTextFile(input.Content)
	change := wholeFileChange(path.display, textFile{}, after, FileChangeCreate)
	if err := s.authorize(ctx, permission.KindFileWrite, path.scope, absoluteTargets(path), "create "+path.display, []FileChange{change}); err != nil {
		return fileWriteOutput{}, err
	}
	approvedPath := path
	path, err = s.resolver.writeTarget(input.Path)
	if err != nil {
		return fileWriteOutput{}, handled(err)
	}
	if path.exists {
		return fileWriteOutput{}, handled(errors.New("file already exists"))
	}
	change = wholeFileChange(path.display, textFile{}, after, FileChangeCreate)
	if !sameTarget(approvedPath, path) {
		if err := s.authorize(ctx, permission.KindFileWrite, path.scope, absoluteTargets(path), "create "+path.display, []FileChange{change}); err != nil {
			return fileWriteOutput{}, err
		}
	}
	if err := createNewFile(path.real, input.Content); err != nil {
		return fileWriteOutput{}, handled(err)
	}
	return fileWriteOutput{
		Path:        path.display,
		Bytes:       len(input.Content),
		Lines:       len(after.lines),
		Revision:    after.revision(),
		FileChanges: []FileChange{change},
	}, nil
}

func (s service) newEditFileTool() (tool.Tool, error) {
	return tool.NewFunc(tool.Definition{
		Name:        "edit_file",
		Description: "Apply non-overlapping hashline edits to an existing UTF-8 file. Use the exact revision and anchors from read_file or search_text.",
	}, s.editFile)
}

func (s service) editFile(ctx context.Context, input editFileInput) (editFileOutput, error) {
	plan, err := s.planEdit(input)
	if err != nil {
		return editFileOutput{}, handled(err)
	}
	if err := s.authorize(ctx, permission.KindFileWrite, plan.path.scope, absoluteTargets(plan.path), "edit "+plan.path.display, plan.changes); err != nil {
		return editFileOutput{}, err
	}
	approvedPath := plan.path
	// A confirmation can block for an arbitrary duration. Rebuilding the plan
	// verifies both the revision and all anchors immediately before writing.
	plan, err = s.planEdit(input)
	if err != nil {
		return editFileOutput{}, handled(err)
	}
	if !sameTarget(approvedPath, plan.path) {
		if err := s.authorize(ctx, permission.KindFileWrite, plan.path.scope, absoluteTargets(plan.path), "edit "+plan.path.display, plan.changes); err != nil {
			return editFileOutput{}, err
		}
	}
	if err := writeAtomic(plan.path.real, plan.after.String(), plan.path.info); err != nil {
		return editFileOutput{}, handled(err)
	}
	anchors, _, _ := plan.after.anchoredLines(plan.changedStart, plan.changedEnd, defaultMaxChars)
	return editFileOutput{
		Path:        plan.path.display,
		Revision:    plan.after.revision(),
		Anchors:     anchors,
		FileChanges: plan.changes,
	}, nil
}

type editPlan struct {
	path         resolvedPath
	before       textFile
	after        textFile
	changes      []FileChange
	changedStart int
	changedEnd   int
}

type plannedEdit struct {
	operation string
	start     int
	end       int
	content   []string
}

func (s service) planEdit(input editFileInput) (editPlan, error) {
	if strings.TrimSpace(input.ExpectedRevision) == "" {
		return editPlan{}, errors.New("expected_revision must not be empty")
	}
	if len(input.Edits) == 0 {
		return editPlan{}, errors.New("edits must not be empty")
	}
	path, err := s.resolver.existing(input.Path)
	if err != nil {
		return editPlan{}, err
	}
	if path.info.IsDir() {
		return editPlan{}, errors.New("path is a directory")
	}
	before, err := loadTextFile(path.real)
	if err != nil {
		return editPlan{}, err
	}
	if input.ExpectedRevision != before.revision() {
		return editPlan{}, errors.New("stale file revision; read the file again before editing it")
	}
	planned, err := validateEdits(before, input.Edits)
	if err != nil {
		return editPlan{}, err
	}
	after := applyEdits(before, planned)
	changes := editChanges(path.display, before, planned)
	changedStart, changedEnd := changedAnchorRange(planned, after)
	return editPlan{
		path:         path,
		before:       before,
		after:        after,
		changes:      changes,
		changedStart: changedStart,
		changedEnd:   changedEnd,
	}, nil
}

func validateEdits(file textFile, edits []editOperation) ([]plannedEdit, error) {
	planned := make([]plannedEdit, 0, len(edits))
	for _, edit := range edits {
		operation := strings.ToLower(strings.TrimSpace(edit.Operation))
		plannedEdit := plannedEdit{operation: operation}
		switch operation {
		case "replace", "delete":
			if strings.TrimSpace(edit.Start) == "" {
				return nil, fmt.Errorf("%s edit requires start", operation)
			}
			start, err := file.verifyAnchor(edit.Start)
			if err != nil {
				return nil, err
			}
			end := start
			if strings.TrimSpace(edit.End) != "" {
				end, err = file.verifyAnchor(edit.End)
				if err != nil {
					return nil, err
				}
			}
			if end < start {
				return nil, errors.New("edit end must not precede start")
			}
			plannedEdit.start, plannedEdit.end = start, end
			if operation == "replace" {
				plannedEdit.content = splitEditContent(edit.Content)
			}
		case "insert_before", "insert_after":
			if strings.TrimSpace(edit.Anchor) == "" {
				return nil, fmt.Errorf("%s edit requires anchor", operation)
			}
			anchor, err := file.verifyAnchor(edit.Anchor)
			if err != nil {
				return nil, err
			}
			plannedEdit.start, plannedEdit.end = anchor, anchor
			plannedEdit.content = splitEditContent(edit.Content)
		default:
			return nil, fmt.Errorf("unsupported edit operation %q", edit.Operation)
		}
		planned = append(planned, plannedEdit)
	}

	sort.Slice(planned, func(i, j int) bool {
		if planned[i].start == planned[j].start {
			return planned[i].end < planned[j].end
		}
		return planned[i].start < planned[j].start
	})
	for index := 1; index < len(planned); index++ {
		if planned[index].start <= planned[index-1].end {
			return nil, errors.New("hashline edits must not overlap or target the same anchor")
		}
	}
	return planned, nil
}

func applyEdits(file textFile, edits []plannedEdit) textFile {
	result := textFile{lines: slices.Clone(file.lines), trailingNewline: file.trailingNewline}
	for index := len(edits) - 1; index >= 0; index-- {
		edit := edits[index]
		switch edit.operation {
		case "replace":
			result.lines = replaceLines(result.lines, edit.start-1, edit.end, edit.content)
		case "delete":
			result.lines = replaceLines(result.lines, edit.start-1, edit.end, nil)
		case "insert_before":
			result.lines = replaceLines(result.lines, edit.start-1, edit.start-1, edit.content)
		case "insert_after":
			result.lines = replaceLines(result.lines, edit.start, edit.start, edit.content)
		}
	}
	return result
}

func replaceLines(lines []string, start, end int, replacement []string) []string {
	result := make([]string, 0, len(lines)-(end-start)+len(replacement))
	result = append(result, lines[:start]...)
	result = append(result, replacement...)
	result = append(result, lines[end:]...)
	return result
}

func editChanges(path string, before textFile, edits []plannedEdit) []FileChange {
	hunks := make([]DiffHunk, 0, len(edits))
	delta := 0
	for _, edit := range edits {
		oldStart, oldEnd := edit.start, edit.end
		newStart := oldStart + delta
		var removed, added []string
		switch edit.operation {
		case "replace":
			removed, added = before.lines[oldStart-1:oldEnd], edit.content
			delta += len(added) - len(removed)
		case "delete":
			removed = before.lines[oldStart-1 : oldEnd]
			delta -= len(removed)
		case "insert_before":
			oldEnd = oldStart - 1
			added = edit.content
			delta += len(added)
		case "insert_after":
			oldStart, oldEnd = edit.start+1, edit.start
			newStart++
			added = edit.content
			delta += len(added)
		}
		hunks = append(hunks, makeHunk(before.lines, oldStart, oldEnd, newStart, removed, added))
	}
	change := FileChange{Path: path, Kind: FileChangeUpdate, Hunks: hunks}
	return []FileChange{truncateChange(change)}
}

func changedAnchorRange(edits []plannedEdit, after textFile) (int, int) {
	if len(after.lines) == 0 {
		return 0, 0
	}
	start, end := len(after.lines), 1
	delta := 0
	for _, edit := range edits {
		position := edit.start + delta
		count := len(edit.content)
		if edit.operation == "delete" {
			count = 0
		}
		if edit.operation == "insert_after" {
			position++
		}
		if count > 0 {
			start = min(start, position)
			end = max(end, position+count-1)
		}
		switch edit.operation {
		case "replace":
			delta += len(edit.content) - (edit.end - edit.start + 1)
		case "delete":
			delta -= edit.end - edit.start + 1
		default:
			delta += len(edit.content)
		}
	}
	if start > end {
		start = min(len(after.lines), max(1, edits[0].start))
		end = start
	}
	return max(1, start-defaultDiffContext), min(len(after.lines), end+defaultDiffContext)
}

func wholeFileChange(path string, before, after textFile, kind FileChangeKind) FileChange {
	oldStart := 0
	for oldStart < len(before.lines) && oldStart < len(after.lines) && before.lines[oldStart] == after.lines[oldStart] {
		oldStart++
	}
	oldEnd, newEnd := len(before.lines)-1, len(after.lines)-1
	for oldEnd >= oldStart && newEnd >= oldStart && before.lines[oldEnd] == after.lines[newEnd] {
		oldEnd--
		newEnd--
	}
	if oldStart > oldEnd && oldStart > newEnd {
		return FileChange{Path: path, Kind: kind}
	}
	removed := before.lines[oldStart : oldEnd+1]
	added := after.lines[oldStart : newEnd+1]
	change := FileChange{
		Path:  path,
		Kind:  kind,
		Hunks: []DiffHunk{makeHunk(before.lines, oldStart+1, oldEnd+1, oldStart+1, removed, added)},
	}
	return truncateChange(change)
}

func makeHunk(before []string, oldStart, oldEnd, newStart int, removed, added []string) DiffHunk {
	contextStart := min(max(oldStart-defaultDiffContext, 1), len(before)+1)
	contextEnd := min(len(before), oldEnd+defaultDiffContext)
	lines := make([]DiffLine, 0, contextEnd-contextStart+1+len(removed)+len(added))
	newLine := newStart - (oldStart - contextStart)
	for oldLine := contextStart; oldLine < oldStart; oldLine++ {
		lines = append(lines, DiffLine{Kind: DiffLineContext, OldLine: oldLine, NewLine: newLine, Content: before[oldLine-1]})
		newLine++
	}
	for index, content := range removed {
		lines = append(lines, DiffLine{Kind: DiffLineRemoved, OldLine: oldStart + index, Content: content})
	}
	for index, content := range added {
		lines = append(lines, DiffLine{Kind: DiffLineAdded, NewLine: newLine + index, Content: content})
	}
	newLine += len(added)
	for oldLine := oldEnd + 1; oldLine <= contextEnd; oldLine++ {
		lines = append(lines, DiffLine{Kind: DiffLineContext, OldLine: oldLine, NewLine: newLine, Content: before[oldLine-1]})
		newLine++
	}
	return DiffHunk{OldStart: contextStart, NewStart: newStart - (oldStart - contextStart), Lines: lines}
}

func truncateChange(change FileChange) FileChange {
	const maxDiffLines = 400
	used := 0
	for hunkIndex := range change.Hunks {
		hunk := &change.Hunks[hunkIndex]
		if used+len(hunk.Lines) <= maxDiffLines {
			used += len(hunk.Lines)
			continue
		}
		remaining := max(0, maxDiffLines-used)
		hunk.Lines = hunk.Lines[:remaining]
		change.Hunks = change.Hunks[:hunkIndex+1]
		change.Truncated = true
		break
	}
	return change
}

func fileChangeKind(exists bool) FileChangeKind {
	if exists {
		return FileChangeUpdate
	}
	return FileChangeCreate
}

func loadTextFile(path string) (textFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return textFile{}, err
	}
	if info.Size() > maxTextFileBytes {
		return textFile{}, fmt.Errorf("file exceeds %d byte read limit", maxTextFileBytes)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return textFile{}, err
	}
	if !utf8.Valid(content) || strings.IndexByte(string(content), 0) >= 0 {
		return textFile{}, errors.New("file is not UTF-8 text")
	}
	return parseTextFile(string(content)), nil
}

func loadTextFileIfExists(path string) (textFile, error) {
	file, err := loadTextFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return textFile{}, nil
	}
	return file, err
}

func writeAtomic(path, content string, existing fs.FileInfo) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directories: %w", err)
	}
	mode := fs.FileMode(0o644)
	if existing != nil {
		mode = existing.Mode().Perm()
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".koda-write-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath) //nolint:errcheck // Rename removes it on success.
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close() //nolint:errcheck // Preserve the chmod error.
		return fmt.Errorf("set temporary file mode: %w", err)
	}
	if _, err := temporary.WriteString(content); err != nil {
		temporary.Close() //nolint:errcheck // Preserve the write error.
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}

func createNewFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directories: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close() //nolint:errcheck // The explicit close below reports errors.
	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("write new file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close new file: %w", err)
	}
	return nil
}

func handled(err error) error {
	if err == nil {
		return nil
	}
	return tool.NewHandledError(err.Error())
}

func sameTarget(left, right resolvedPath) bool {
	return left.real == right.real && left.scope == right.scope
}

func clamp(value, fallback, maximum int) int {
	if value <= 0 {
		return fallback
	}
	return min(value, maximum)
}
