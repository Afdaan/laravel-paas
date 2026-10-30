export function BuildLogLine({ line }: { line: string }) {
  const trimmed = line.trim()
  const lower = trimmed.toLowerCase()

  if (
    trimmed.startsWith('✗') ||
    /\[(?:FAILED|ERROR|[A-Z_]*FAILED)\]|(?:SyntaxError|TypeError|ReferenceError|RangeError|ParseError):|npm (?:ERR!|error\b)/i.test(trimmed) ||
    ((lower.includes('error:') || lower.includes('failed')) && !lower.includes('success'))
  ) {
    return <span className="font-medium text-rose-300">{line}</span>
  }

  if (trimmed.startsWith('>>')) {
    const markerIndex = line.indexOf('>>')
    const prefix = line.substring(0, markerIndex)
    const content = line.substring(markerIndex + 2)
    return (
      <span className="font-medium text-sky-300">
        {prefix && <span className="text-zinc-500">{prefix}</span>}
        <span className="mr-1.5 select-none text-sky-500">&gt;&gt;</span>
        <span>{content}</span>
      </span>
    )
  }

  if (trimmed.startsWith('✓')) {
    const markerIndex = line.indexOf('✓')
    const prefix = line.substring(0, markerIndex)
    const content = line.substring(markerIndex + 1)
    return (
      <span className="font-medium text-emerald-300">
        {prefix && <span className="text-zinc-500">{prefix}</span>}
        <span className="mr-1.5 select-none font-bold text-emerald-500">✓</span>
        <span>{content}</span>
      </span>
    )
  }

  if (trimmed.startsWith('[SUCCESS]') || (trimmed.startsWith('[BUILD SUMMARY]') && lower.includes('successfully'))) {
    return <span className="font-medium text-emerald-300">{line}</span>
  }

  if (trimmed.startsWith('[WARNING]') || lower.includes('warning:')) {
    return <span className="font-medium text-amber-300">{line}</span>
  }

  if (trimmed.startsWith('===') || trimmed.startsWith('---')) {
    return <span className="select-none font-light tracking-widest text-zinc-600">{line}</span>
  }

  if (trimmed.startsWith('INFO')) {
    return (
      <span className="text-zinc-300">
        <span className="mr-1.5 font-medium text-zinc-500">INFO</span>
        {line.substring(4)}
      </span>
    )
  }

  if (trimmed.startsWith('> ')) {
    return <span className="text-cyan-300">{line}</span>
  }

  if (trimmed.startsWith('Dependencies:')) {
    return <span className="text-teal-200/90">{line}</span>
  }

  return <span>{line}</span>
}
