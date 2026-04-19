import { useEffect, useRef } from 'react'

/**
 * usePolling executes a callback at a specified interval.
 * It ensures the interval is cleared on unmount and uses a ref 
 * to always call the latest version of the callback.
 */
export function usePolling(callback: () => void, delay: number | null) {
  const savedCallback = useRef(callback)

  // Remember the latest callback
  useEffect(() => {
    savedCallback.current = callback
  }, [callback])

  // Set up the interval
  useEffect(() => {
    if (delay !== null) {
      const id = setInterval(() => savedCallback.current(), delay)
      return () => clearInterval(id)
    }
  }, [delay])
}
