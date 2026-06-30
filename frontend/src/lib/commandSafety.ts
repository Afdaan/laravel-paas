const riskyCommandPatterns = [
  /^migrate:fresh(?:\s|$)/,
  /^migrate:reset(?:\s|$)/,
  /^migrate:rollback(?:\s|$)/,
  /^db:wipe(?:\s|$)/,
  /^schema:dump(?:\s|$)/,
  /^queue:flush(?:\s|$)/,
  /^queue:forget(?:\s|$)/,
  /(?:^|\s)rm\s+-[^\n]*[rf]/,
  /(?:^|\s)(npm|pnpm|yarn|bun)\s+(publish|unpublish)(?:\s|$)/,
  /(?:^|\s)(npm|pnpm|yarn|bun)\s+(install|add|remove|update|upgrade)(?:\s|$)/,
  /(?:^|\s)composer\s+(install|update|remove)(?:\s|$)/,
  /(?:^|\s)cargo\s+publish(?:\s|$)/,
]

const shellEvalPatterns = [
  /(?:^|\s)(bash|sh|zsh|dash)\s+-c\b/,
  /(?:^|\s)(curl|wget)\s+.*\|/,
  /(?:^|\s)(python|python3)\s+-c\b/,
  /(?:^|\s)node\s+-e\b/,
  /(?:^|\s)php\s+-r\b/,
  /(?:^|\s)ruby\s+-e\b/,
  /(?:^|\s)perl\s+-e\b/,
]

export type CommandWarningType = 'app-risky' | 'shell-eval'

export function needsCommandConfirmation(command: string) {
  const normalized = command.trim().toLowerCase()
  return riskyCommandPatterns.some(pattern => pattern.test(normalized)) ||
    shellEvalPatterns.some(pattern => pattern.test(normalized))
}

export function commandWarningType(command: string): CommandWarningType {
  const normalized = command.trim().toLowerCase()
  if (shellEvalPatterns.some(pattern => pattern.test(normalized))) return 'shell-eval'
  return 'app-risky'
}
