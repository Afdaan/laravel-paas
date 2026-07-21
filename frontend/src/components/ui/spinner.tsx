import { cn } from '@/lib/utils'

// Radial bar spinner (v-spinner-14 style): 12 bars fade in sequence around a
// circle. Pure CSS via the `spinner-bar` keyframe in index.css — no JS ticker.
const BARS = Array.from({ length: 12 }, (_, i) => i)

export function Spinner({ className }: { className?: string }) {
  return (
    <span
      role="status"
      aria-label="Loading"
      className={cn('relative inline-block size-4 shrink-0', className)}
    >
      {BARS.map((i) => (
        <span
          key={i}
          className="absolute left-1/2 top-0 h-[30%] w-[8%] -translate-x-1/2 rounded-full bg-current"
          style={{
            transformOrigin: 'center 166%',
            transform: `rotate(${i * 30}deg)`,
            animation: 'spinner-bar 1s linear infinite',
            animationDelay: `${(i - 12) / 12}s`,
          }}
        />
      ))}
    </span>
  )
}
