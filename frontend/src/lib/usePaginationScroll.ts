import { useCallback, useRef } from 'react'

import { scheduleScrollElementIntoMain } from '@/lib/scrollIntoMain'

export function usePaginationScroll<T extends HTMLElement = HTMLDivElement>() {
  const paginationAnchorRef = useRef<T>(null)
  const updatePagination = useCallback((update: () => void) => {
    const anchor = paginationAnchorRef.current
    update()
    if (anchor) scheduleScrollElementIntoMain(anchor)
  }, [])

  return { paginationAnchorRef, updatePagination }
}
