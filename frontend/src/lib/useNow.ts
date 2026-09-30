import { useEffect, useState } from 'react'

/**
 * Ticks once a second while `active`, so callers can derive elapsed time
 * without each of them owning a timer.
 *
 * Pauses while the tab is hidden and resyncs on focus — a background tab
 * throttles intervals, so the clock would otherwise drift behind and jump on
 * return. Mirrors the visibility handling in Login.tsx's rate-limit countdown,
 * which also listens for `focus` (usePolling does not).
 */
export function useNow(active: boolean): number {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!active) return

    const sync = () => setNow(Date.now())
    sync()

    let interval: number | undefined
    const start = () => {
      if (interval === undefined) interval = window.setInterval(sync, 1_000)
    }
    const stop = () => {
      if (interval !== undefined) {
        window.clearInterval(interval)
        interval = undefined
      }
    }

    const handleVisibility = () => {
      if (document.visibilityState === 'visible') {
        sync()
        start()
      } else {
        stop()
      }
    }

    if (document.visibilityState === 'visible') start()
    document.addEventListener('visibilitychange', handleVisibility)
    window.addEventListener('focus', sync)

    return () => {
      stop()
      document.removeEventListener('visibilitychange', handleVisibility)
      window.removeEventListener('focus', sync)
    }
  }, [active])

  return now
}

export default useNow
