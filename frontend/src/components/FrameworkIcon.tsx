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

function ViteLogo() {
  return (
    <svg viewBox="0 0 256 257" className="h-full w-full" xmlns="http://www.w3.org/2000/svg" preserveAspectRatio="xMidYMid meet">
      <defs>
        <linearGradient id="viteGradient" x1="-0.11%" x2="101.14%" y1="100%" y2="0%">
          <stop offset="0%" stopColor="#41D1FF" />
          <stop offset="100%" stopColor="#BD34FE" />
        </linearGradient>
        <linearGradient id="boltGradient" x1="-12.1%" x2="89.63%" y1="50%" y2="50%">
          <stop offset="0%" stopColor="#FFEA83" />
          <stop offset="8.33%" stopColor="#FFDD35" />
          <stop offset="100%" stopColor="#FFA800" />
        </linearGradient>
      </defs>
      <path fill="url(#viteGradient)" d="M255.859 37.835L128 257L0.141 37.835L128 0l127.859 37.835z" />
      <path fill="url(#boltGradient)" d="M150.5 0L128 77l-22.5-77L28 0l100 257L228 0h-77.5z" />
    </svg>
  )
}

export function FrameworkIcon({ framework, className, variant = 'tile' }: FrameworkIconProps) {
  const fw = normalizeFramework(framework)
  const icon = getFrameworkIcon(framework)

  const variants: Record<NonNullable<FrameworkIconProps['variant']>, string> = {
    tile: 'rounded-xl bg-muted/50 p-2.5 ring-1 ring-border/70 shadow-sm dark:bg-muted/20 dark:ring-white/10 [&>svg]:h-full [&>svg]:w-full',
    compact: 'rounded-lg bg-muted/40 p-1 ring-1 ring-border/60',
    plain: 'rounded-none bg-transparent p-0 ring-0 shadow-none',
  }

  // Special case for Vite (Premium Logo)
  if (fw.includes('vite')) {
    return (
      <div className={cn('flex items-center justify-center', variants[variant], className)}>
        <ViteLogo />
      </div>
    )
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
