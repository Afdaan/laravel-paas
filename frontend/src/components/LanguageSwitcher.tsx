import { Languages } from 'lucide-react'
import useTranslation from '@/lib/useTranslation'
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

export function LanguageSwitcher() {
  const { language, setLanguage } = useTranslation()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="group inline-flex size-8 items-center justify-center rounded-lg border border-border bg-background transition-all duration-200 hover:bg-muted hover:text-foreground hover:shadow-sm hover:shadow-primary/5 aria-expanded:bg-muted aria-expanded:text-foreground dark:border-input dark:bg-input/30 dark:hover:bg-input/50 cursor-pointer focus:outline-none focus:ring-2 focus:ring-primary/20">
        <Languages className="h-3.5 w-3.5 text-muted-foreground transition-colors group-hover:text-primary" />
        <span className="sr-only">Toggle language</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56 p-1.5 animate-in fade-in-0 zoom-in-95">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="px-2 py-1.5 text-xs font-bold uppercase tracking-widest text-muted-foreground/70">
            Select Language
          </DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator className="opacity-50" />
        <DropdownMenuRadioGroup value={language} onValueChange={(val) => setLanguage(val as 'en' | 'id')}>
          <DropdownMenuRadioItem 
            value="en"
            className="flex items-center justify-between rounded-md px-2 py-2.5 transition-colors focus:bg-primary/5 data-[state=checked]:bg-primary/5"
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
            className="flex items-center justify-between rounded-md px-2 py-2.5 transition-colors focus:bg-primary/5 data-[state=checked]:bg-primary/5"
          >
            <div className="flex items-center gap-3">
              <div className="flex size-6 items-center justify-center rounded bg-primary/10 text-[10px] font-bold uppercase text-primary ring-1 ring-primary/20">
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
