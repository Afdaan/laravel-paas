export const MAX_RETAINED_BUILD_LOG_LINES = 1500

export type BuildLogsState = {
  lines: string[]
  clearedCount: number
}

export type BuildLogsSnapshot = {
  lines: string[]
  available: boolean
}

export const initialBuildLogsState: BuildLogsState = {
  lines: [],
  clearedCount: 0,
}

export const splitLogSnapshot = (value: string) => {
  return value.split('\n').filter((line: string) => line.trim() !== '' || line === '')
}

const capLogState = (state: BuildLogsState): BuildLogsState => {
  if (state.lines.length <= MAX_RETAINED_BUILD_LOG_LINES) {
    return state
  }

  const droppedCount = state.lines.length - MAX_RETAINED_BUILD_LOG_LINES
  return {
    lines: state.lines.slice(droppedCount),
    clearedCount: Math.max(0, state.clearedCount - droppedCount),
  }
}

const hasSameTail = (current: string[], next: string[]) => {
  if (next.length === 0 || current.length < next.length) {
    return false
  }

  const offset = current.length - next.length
  for (let index = 0; index < next.length; index += 1) {
    if (current[offset + index] !== next[index]) {
      return false
    }
  }

  return true
}

const hasSamePrefix = (current: string[], next: string[]) => {
  if (next.length === 0 || current.length < next.length) {
    return false
  }

  for (let index = 0; index < next.length; index += 1) {
    if (current[index] !== next[index]) {
      return false
    }
  }

  return true
}

const capSnapshotLines = (lines: string[]) => {
  return lines.length > MAX_RETAINED_BUILD_LOG_LINES
    ? lines.slice(-MAX_RETAINED_BUILD_LOG_LINES)
    : lines
}

const buildPrefixTable = (lines: string[]) => {
  const table = new Array<number>(lines.length).fill(0)
  let length = 0

  for (let index = 1; index < lines.length; index += 1) {
    while (length > 0 && lines[index] !== lines[length]) {
      length = table[length - 1]
    }

    if (lines[index] === lines[length]) {
      length += 1
    }

    table[index] = length
  }

  return table
}

const findSuffixPrefixOverlap = (source: string[], target: string[]) => {
  if (source.length === 0 || target.length === 0) {
    return 0
  }

  const prefixTable = buildPrefixTable(target)
  let matched = 0

  for (const line of source) {
    while (matched > 0 && line !== target[matched]) {
      matched = prefixTable[matched - 1]
    }

    if (line === target[matched]) {
      matched += 1
    }

    if (matched === target.length) {
      matched = prefixTable[matched - 1]
    }
  }

  return matched
}

export const mergeBuildLogSnapshot = (
  state: BuildLogsState,
  snapshot: BuildLogsSnapshot,
): BuildLogsState => {
  if (!snapshot.available) {
    return state.lines.length > 0 ? state : initialBuildLogsState
  }

  if (snapshot.lines.length === 0) {
    return state.lines.length > 0 ? state : initialBuildLogsState
  }

  const snapshotLines = capSnapshotLines(snapshot.lines)

  if (hasSameTail(state.lines, snapshotLines)) {
    return state
  }

  if (hasSamePrefix(state.lines, snapshotLines)) {
    return state
  }

  const overlap = findSuffixPrefixOverlap(state.lines, snapshotLines)
  if (overlap > 0) {
    return capLogState({
      lines: [...state.lines, ...snapshotLines.slice(overlap)],
      clearedCount: state.clearedCount,
    })
  }

  if (findSuffixPrefixOverlap(snapshotLines, state.lines) > 0) {
    return state
  }

  return capLogState({
    lines: snapshotLines,
    clearedCount: Math.min(state.clearedCount, snapshotLines.length),
  })
}

export const appendBuildLogLines = (
  state: BuildLogsState,
  lines: string[],
): BuildLogsState => {
  if (lines.length === 0) {
    return state
  }

  return capLogState({
    lines: [...state.lines, ...lines],
    clearedCount: state.clearedCount,
  })
}

export const clearVisibleBuildLogs = (state: BuildLogsState): BuildLogsState => {
  return {
    ...state,
    clearedCount: state.lines.length,
  }
}
