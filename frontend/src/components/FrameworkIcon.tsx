import { Globe, Code2 } from 'lucide-react'
import {
  siVite,
  siLaravel,
  siNextdotjs,
  siReact,
  siJavascript,
  siTypescript,
  siNodedotjs,
  siPython,
  siVuedotjs,
  siGo,
  siPhp,
  siNuxt,
  siSvelte,
  siDjango,
  siFlask,
  type SimpleIcon,
} from 'simple-icons'
import { cn } from '@/lib/utils'

interface FrameworkIconProps {
  framework?: string
  className?: string
  variant?: 'tile' | 'compact' | 'plain'
}

function normalizeFramework(framework?: string) {
  return (framework || '').toLowerCase().trim()
}

function getFrameworkIcon(framework?: string): SimpleIcon | null {
  const fw = normalizeFramework(framework)

  if (fw.includes('laravel')) return siLaravel
  if (fw.includes('vite')) return siVite
  if (fw.includes('next')) return siNextdotjs
  if (fw.includes('react')) return siReact
  if (fw.includes('vue')) return siVuedotjs
  if (fw.includes('nuxt')) return siNuxt
  if (fw.includes('svelte')) return siSvelte
  if (fw.includes('node')) return siNodedotjs
  if (fw.includes('python')) return siPython
  if (fw.includes('django')) return siDjango
  if (fw.includes('flask')) return siFlask
  if (fw === 'js' || fw.includes('javascript')) return siJavascript
  if (fw === 'ts' || fw.includes('typescript')) return siTypescript
  if (fw.includes('php')) return siPhp
  if (fw === 'go' || fw.includes('golang')) return siGo

  return null
}

function BrandGlyph({ icon }: { icon: SimpleIcon }) {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" className="h-full w-full" xmlns="http://www.w3.org/2000/svg">
      <path fill={`#${icon.hex}`} d={icon.path} />
    </svg>
  )
}

export function FrameworkIcon({ framework, className, variant = 'tile' }: FrameworkIconProps) {
  const fw = normalizeFramework(framework)
  const icon = getFrameworkIcon(framework)

  const variants: Record<NonNullable<FrameworkIconProps['variant']>, string> = {
    tile: 'rounded-2xl bg-muted/60 dark:bg-zinc-950/80 p-1.5 ring-1 ring-border dark:ring-white/10 shadow-sm dark:shadow-[inset_0_1px_0_rgba(255,255,255,0.06)]',
    compact: 'rounded-lg bg-muted/40 p-1 ring-1 ring-border/60',
    plain: 'rounded-none bg-transparent p-0 ring-0 shadow-none',
  }

  if (icon) {
    return (
      <div
        className={cn(
          'flex items-center justify-center',
          variants[variant],
          className,
        )}
      >
        <BrandGlyph icon={icon} />
      </div>
    )
  }

  // Static HTML / Generic Code
  if (fw.includes('static') || fw.includes('html')) {
    return (
      <div className={cn('flex items-center justify-center', variants[variant], className)}>
        <Code2 className="w-full h-full text-orange-500" />
      </div>
    )
  }

  return (
    <div className={cn('flex items-center justify-center', variants[variant], className)}>
      <Globe className="w-full h-full text-muted-foreground" />
    </div>
  )
}
