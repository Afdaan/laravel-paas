import { useRef } from 'react'
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { PAGE_SIZES, type PaginationState } from '@/lib/pagination'
import useTranslation from '@/lib/useTranslation'

export function TablePagination({
  state,
  disabled = false,
  className = '',
}: {
  state: PaginationState
  disabled?: boolean
  className?: string
}) {
  const { t } = useTranslation()
  const { page, pageSize, total, totalPages, start, end, setPage, setPageSize } = state
  const containerRef = useRef<HTMLDivElement>(null)
  if (total === 0) return null

  const preservePosition = (update: () => void) => {
    const container = containerRef.current
    if (!container) {
      update()
      return
    }
    const previousTop = container.getBoundingClientRect().top
    update()
    requestAnimationFrame(() => {
      const offset = container.getBoundingClientRect().top - previousTop
      if (offset !== 0) window.scrollBy(0, offset)
    })
  }

  return (
    <div
      ref={containerRef}
      className={`flex flex-col gap-3 border-t border-border/40 bg-muted/20 px-4 py-3 sm:flex-row sm:items-center sm:justify-between ${className}`}
    >
      <div className="flex items-center gap-3">
        <span className="text-[11px] tabular-nums text-muted-foreground">
          {start + 1}&ndash;{Math.min(end, total)} / {total}
        </span>
        <div className="flex items-center gap-2">
          <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
            {t('common.rowsPerPage')}
          </span>
          <Select value={pageSize.toString()} onValueChange={(value) => preservePosition(() => setPageSize(Number(value)))} disabled={disabled}>
            <SelectTrigger size="sm" className="h-7 w-[70px] justify-between text-xs">
              <SelectValue placeholder={pageSize} />
            </SelectTrigger>
            <SelectContent side="top" align="start" alignItemWithTrigger={false}>
              {PAGE_SIZES.map((size) => (
                <SelectItem key={size} value={`${size}`} className="text-xs">
                  {size}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      <div className="flex items-center gap-3">
        <span className="text-[11px] tabular-nums text-muted-foreground">
          {page} / {totalPages}
        </span>
        <div className="flex items-center gap-1">
          <Button variant="outline" className="size-7 p-0" onClick={() => preservePosition(() => setPage(1))} disabled={disabled || page === 1}>
            <span className="sr-only">{t('common.first')}</span>
            <ChevronsLeft className="size-3.5" />
          </Button>
          <Button
            variant="outline"
            className="size-7 p-0"
            onClick={() => preservePosition(() => setPage(page - 1))}
            disabled={disabled || page === 1}
          >
            <span className="sr-only">{t('common.previous')}</span>
            <ChevronLeft className="size-3.5" />
          </Button>
          <Button
            variant="outline"
            className="size-7 p-0"
            onClick={() => preservePosition(() => setPage(page + 1))}
            disabled={disabled || page >= totalPages}
          >
            <span className="sr-only">{t('common.next')}</span>
            <ChevronRight className="size-3.5" />
          </Button>
          <Button
            variant="outline"
            className="size-7 p-0"
            onClick={() => preservePosition(() => setPage(totalPages))}
            disabled={disabled || page >= totalPages}
          >
            <span className="sr-only">{t('common.last')}</span>
            <ChevronsRight className="size-3.5" />
          </Button>
        </div>
      </div>
    </div>
  )
}
