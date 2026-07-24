import { Languages } from 'lucide-react'
import useTranslation from '@/lib/useTranslation'
import { cn } from '@/lib/utils'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export function LanguageSwitcher({ className, forceDark = false }: { className?: string; forceDark?: boolean }) {
  const { t, language, setLanguage } = useTranslation()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className={cn("group inline-flex size-8 cursor-pointer items-center justify-center rounded-lg border border-border bg-background transition-colors duration-150 hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring", className)}>
        <Languages className="h-3.5 w-3.5 text-muted-foreground transition-colors group-hover:text-primary" aria-hidden="true" />
        <span className="sr-only">{t('landing.language.toggle')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className={cn("w-56 p-1.5 animate-in fade-in-0 zoom-in-95", forceDark && "runara-surface bg-popover text-popover-foreground ring-border")}>
        <DropdownMenuGroup>
          <DropdownMenuLabel className="px-2 py-1.5 text-xs font-bold uppercase tracking-widest text-muted-foreground/70">
            {t('landing.language.label')}
          </DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator className="opacity-50" />
        <DropdownMenuRadioGroup value={language} onValueChange={(val) => setLanguage(val as 'en' | 'id')}>
          <DropdownMenuRadioItem
            value="en"
            className="flex min-h-11 items-center justify-between rounded-md px-2 py-3 transition-colors focus:bg-primary/5 data-checked:bg-primary/5"
          >
            <div className="flex items-center gap-3">
              <div className="flex size-6 items-center justify-center rounded bg-muted text-[10px] font-bold uppercase text-muted-foreground ring-1 ring-border">
                EN
              </div>
              <span className="text-sm font-medium tracking-tight">English</span>
            </div>
          </DropdownMenuRadioItem>

          <DropdownMenuRadioItem
            value="id"
            className="flex min-h-11 items-center justify-between rounded-md px-2 py-3 transition-colors focus:bg-primary/5 data-checked:bg-primary/5"
          >
            <div className="flex items-center gap-3">
              <div className="flex size-6 items-center justify-center rounded bg-muted text-[10px] font-bold uppercase text-muted-foreground ring-1 ring-border">
                ID
              </div>
              <span className="text-sm font-medium tracking-tight">Bahasa Indonesia</span>
            </div>
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
