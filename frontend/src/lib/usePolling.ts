import { useEffect, useRef } from 'react'

/**
 * usePolling executes a callback at a specified interval.
 * It pauses polling when the browser tab is inactive (hidden)
 * and resumes immediately when the tab is focused (visible).
 */
export function usePolling(callback: () => void, delay: number | null) {
  const savedCallback = useRef(callback)

  // Remember the latest callback
  useEffect(() => {
    savedCallback.current = callback
  }, [callback])

  // Set up the interval with visibility listeners
  useEffect(() => {
    if (delay === null) return

    let intervalId: ReturnType<typeof setInterval> | null = null

    const startInterval = () => {
      if (!intervalId) {
        intervalId = setInterval(() => {
          savedCallback.current()
        }, delay)
      }
    }

    const stopInterval = () => {
      if (intervalId) {
        clearInterval(intervalId)
        intervalId = null
      }
    }

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        // Refresh immediately on tab focus, then resume interval
        savedCallback.current()
        startInterval()
      } else {
        stopInterval()
      }
    }

    // Only start polling if currently visible
    if (document.visibilityState === 'visible') {
      startInterval()
    }

    document.addEventListener('visibilitychange', handleVisibilityChange)

    return () => {
      stopInterval()
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [delay])
}
