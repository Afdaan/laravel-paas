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
  siAngular,
  siExpress,
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
  if (fw.includes('angular')) return siAngular
  if (fw.includes('express')) return siExpress
  if (fw === 'js' || fw.includes('javascript')) return siJavascript
  if (fw === 'ts' || fw.includes('typescript')) return siTypescript
  if (fw.includes('php')) return siPhp
  if (fw === 'go' || fw.includes('golang')) return siGo

  return null
}

type IconTreatment = {
  useCurrentColor?: boolean
  glyphClassName?: string
  variants?: Record<NonNullable<FrameworkIconProps['variant']>, string>
}

function BrandGlyph({ icon, className, useCurrentColor }: { icon: SimpleIcon; className?: string; useCurrentColor?: boolean }) {
  const fillColor = useCurrentColor ? 'currentColor' : `#${icon.hex}`
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" className={cn('h-full w-full', className)} xmlns="http://www.w3.org/2000/svg">
      <path fill={fillColor} d={icon.path} />
    </svg>
  )
}

function getIconTreatment(icon: SimpleIcon | null): IconTreatment {
  if (!icon) return {}

  if (icon === siNextdotjs || icon === siExpress) {
    return {
      useCurrentColor: true,
      glyphClassName: 'drop-shadow-[0_1px_10px_rgba(255,255,255,0.18)]',
      variants: {
        tile: 'rounded-xl bg-[linear-gradient(145deg,rgba(255,255,255,0.13),rgba(255,255,255,0.045))] p-2.5 text-white ring-1 ring-white/15 shadow-[inset_0_1px_0_rgba(255,255,255,0.12),0_14px_36px_rgba(0,0,0,0.32)]',
        compact: 'rounded-lg bg-white/[0.08] p-1 text-white ring-1 ring-white/15 shadow-sm',
        plain: 'rounded-none bg-transparent p-0 text-foreground ring-0 shadow-none dark:text-white',
      },
    }
  }

  if (icon === siDjango) {
    return {
      useCurrentColor: true,
      glyphClassName: 'drop-shadow-[0_1px_10px_rgba(68,183,139,0.2)]',
      variants: {
        tile: 'rounded-xl bg-emerald-500/[0.08] p-2.5 text-[#44B78B] ring-1 ring-emerald-300/15 shadow-sm',
        compact: 'rounded-lg bg-emerald-500/[0.08] p-1 text-[#44B78B] ring-1 ring-emerald-300/15',
        plain: 'rounded-none bg-transparent p-0 text-[#44B78B] ring-0 shadow-none',
      },
    }
  }

  if (icon === siAngular) {
    return {
      useCurrentColor: true,
      glyphClassName: 'drop-shadow-[0_1px_10px_rgba(255,77,90,0.18)]',
      variants: {
        tile: 'rounded-xl bg-rose-500/[0.08] p-2.5 text-[#ff4d5a] ring-1 ring-rose-300/15 shadow-sm',
        compact: 'rounded-lg bg-rose-500/[0.08] p-1 text-[#ff4d5a] ring-1 ring-rose-300/15',
        plain: 'rounded-none bg-transparent p-0 text-[#ff4d5a] ring-0 shadow-none',
      },
    }
  }

  return {}
}

function PythonLogo() {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" className="h-full w-full drop-shadow-[0_1px_10px_rgba(71,139,202,0.24)]" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <linearGradient id="pythonBlue" x1="4" y1="3" x2="18" y2="13" gradientUnits="userSpaceOnUse">
          <stop stopColor="#4B8BBE" />
          <stop offset="1" stopColor="#306998" />
        </linearGradient>
        <linearGradient id="pythonGold" x1="6" y1="12" x2="20" y2="22" gradientUnits="userSpaceOnUse">
          <stop stopColor="#FFE873" />
          <stop offset="1" stopColor="#FFD43B" />
        </linearGradient>
      </defs>
      <path
        fill="url(#pythonBlue)"
        d="M11.9 2.25c-3.35 0-5.02.86-5.02 2.58v2.05h5.86v1.05H5.55C3.46 7.93 2 9.42 2 12.05c0 2.5 1.34 3.95 3.42 3.95h1.46v-2.52c0-2.05 1.7-3.62 3.72-3.62h4.78c1.75 0 3.14-1.43 3.14-3.14V4.83c0-1.68-1.55-2.58-4.76-2.58H11.9Z"
      />
      <path
        fill="url(#pythonGold)"
        d="M12.1 21.75c3.35 0 5.02-.86 5.02-2.58v-2.05h-5.86v-1.05h7.19c2.09 0 3.55-1.49 3.55-4.12C22 9.45 20.66 8 18.58 8h-1.46v2.52c0 2.05-1.7 3.62-3.72 3.62H8.62c-1.75 0-3.14 1.43-3.14 3.14v1.89c0 1.68 1.55 2.58 4.76 2.58h1.86Z"
      />
      <circle cx="9.15" cy="4.75" r="0.72" fill="rgba(255,255,255,0.92)" />
      <circle cx="14.85" cy="19.25" r="0.72" fill="rgba(17,24,39,0.7)" />
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
  const treatment = getIconTreatment(icon)

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

  if (icon === siPython) {
    return (
      <div className={cn('flex items-center justify-center', variants[variant], className)}>
        <PythonLogo />
      </div>
    )
  }

  if (icon) {
    return (
      <div
        className={cn(
          'flex items-center justify-center',
          treatment.variants?.[variant] ?? variants[variant],
          className,
        )}
      >
        <BrandGlyph icon={icon} className={treatment.glyphClassName} useCurrentColor={treatment.useCurrentColor} />
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
