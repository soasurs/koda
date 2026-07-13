export type ShellOutput = {
  exitCode: number
  stderr: string
  stdout: string
  truncated: boolean
}

export function parseShellOutput(structured: string, fallback: string) {
  for (const value of [structured, fallback]) {
    if (!value) continue
    try {
      const parsed = JSON.parse(value) as Record<string, unknown>
      if (
        typeof parsed.stdout === 'string' &&
        typeof parsed.stderr === 'string' &&
        typeof parsed.exit_code === 'number'
      ) {
        return {
          exitCode: parsed.exit_code,
          stderr: parsed.stderr,
          stdout: parsed.stdout,
          truncated: parsed.truncated === true,
        } satisfies ShellOutput
      }
    } catch {
      // Koda normally sends structured JSON; ignore an incompatible fallback.
    }
  }
  return null
}
