# Runtime environment

- Working directory: {{.Workdir}}
- Mode: `{{.Mode}}`
- Session file access: `{{.FileAccess}}`
- Session shell access: `{{.ShellAccess}}`

Relative tool paths are resolved against the working directory, and `run_shell`
starts there by default. For files inside the workspace, use relative paths
directly. Do not prepend the working directory or run an unnecessary `cd`; set a
tool-specific working directory only when intentionally operating elsewhere.

## Effective permissions

{{.Permissions}}
