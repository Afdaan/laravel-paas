import { useState } from 'react'

export const PAGE_SIZES = [10, 20, 30, 50, 100]

export type PaginationState = {
  page: number
  pageSize: number
  total: number
  totalPages: number
  start: number
  end: number
  setPage: (page: number) => void
  setPageSize: (size: number) => void
}

// Clamp rather than reset via effect, so a shrinking filter can't strand the
// view on an empty page.
function build(
  page: number,
  pageSize: number,
  total: number,
  setPage: (page: number) => void,
  setPageSize: (size: number) => void,
): PaginationState {
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const current = Math.min(page, totalPages)
  const start = (current - 1) * pageSize
  return {
    page: current,
    pageSize,
    total,
    totalPages,
    start,
    end: start + pageSize,
    setPage,
    setPageSize: (size) => {
      setPageSize(size)
      setPage(1)
    },
  }
}

/** Client-side paging: caller slices its own array with `start`/`end`. */
export function usePagination(total: number, initialSize = 10): PaginationState {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(initialSize)
  return build(page, pageSize, total, setPage, setPageSize)
}

/**
 * Server-side paging: the API returns the slice, so `start`/`end` are display
 * offsets only. Pass the page/size state you already send with the request.
 */
export function serverPagination(
  page: number,
  pageSize: number,
  total: number,
  setPage: (page: number) => void,
  setPageSize: (size: number) => void,
): PaginationState {
  return build(page, pageSize, total, setPage, setPageSize)
}
