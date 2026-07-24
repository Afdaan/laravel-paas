import { motion, useReducedMotion } from 'framer-motion'
import useTranslation from '@/lib/useTranslation'

// Scripted tetris fill: blocks drop into preset slots on a 4x4 well, staggered,
// then the whole stack loops. No physics engine — predetermined coordinates.
const CELL = 16
const GAP = 3
const COLS = 4
const ROWS = 4

// Fill the well bottom-up, row by row, so it reads as a stack completing.
const blocks: Array<[number, number]> = [
  [0, 3], [1, 3], [2, 3], [3, 3],
  [0, 2], [1, 2], [2, 2], [3, 2],
  [0, 1], [1, 1], [2, 1], [3, 1],
  [0, 0], [1, 0], [2, 0], [3, 0],
]

const step = CELL + GAP
const wellWidth = COLS * step - GAP
const wellHeight = ROWS * step - GAP

export default function LoadingScreen() {
  const reduceMotion = useReducedMotion()
  const { t } = useTranslation()

  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-6 bg-background font-sans antialiased">
      <div className="relative" style={{ width: wellWidth, height: wellHeight }}>
        {blocks.map(([col, row], i) => {
          const left = col * step
          const top = row * step
          return (
            <motion.span
              key={i}
              className="absolute rounded-[3px] bg-primary"
              style={{ width: CELL, height: CELL, left }}
              initial={reduceMotion ? { top, opacity: 1 } : { top: -step, opacity: 0 }}
              animate={
                reduceMotion
                  ? { top, opacity: 1 }
                  : { top: [-step, top, top, -step], opacity: [0, 1, 1, 0] }
              }
              transition={
                reduceMotion
                  ? undefined
                  : {
                      duration: 3,
                      times: [0, 0.2, 0.9, 1],
                      repeat: Infinity,
                      ease: 'easeInOut',
                      delay: i * 0.1,
                    }
              }
            />
          )
        })}
      </div>
      <motion.p
        className="text-sm font-medium tracking-wide text-muted-foreground"
        animate={reduceMotion ? undefined : { opacity: [0.5, 1, 0.5] }}
        transition={{ duration: 1.6, repeat: Infinity, ease: 'easeInOut' }}
      >
        {t('common.loading')}
      </motion.p>
    </div>
  )
}
