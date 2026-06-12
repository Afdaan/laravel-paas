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

  if (hasSameTail(state.lines, snapshot.lines)) {
    return state
  }

  return capLogState({
    lines: snapshot.lines,
    clearedCount: Math.min(state.clearedCount, snapshot.lines.length),
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
