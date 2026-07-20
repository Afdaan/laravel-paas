import { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { MotionConfig, motion, useReducedMotion } from 'framer-motion'
import { ArrowRight } from 'lucide-react'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'
import { Button } from '@/components/ui/button'
import useTranslation from '@/lib/useTranslation'
import { cn } from '@/lib/utils'
import useAuthStore from '@/stores/authStore'

const navLinks = [
  ['#workflow', 'landing.nav.workflow'],
  ['#features', 'landing.nav.features'],
  ['#security', 'landing.nav.security'],
] as const

const releaseChecks = ['build', 'readiness', 'release'] as const

const capabilityGroups = [
  { key: 'releaseControl', items: ['source', 'queue', 'gate', 'rollback', 'runtime'] },
  { key: 'operate', items: ['logs', 'console', 'workers', 'limits'] },
  { key: 'dataEdge', items: ['databases', 'studio', 'domains', 'configuration'] },
] as const

const supportedStacks = [
  'Laravel', 'Vite', 'Next.js', 'React', 'Node.js', 'Python', 'Vue', 'Go', 'PHP',
  'Nuxt', 'Svelte', 'Django', 'Flask', 'Angular', 'Express', 'JavaScript', 'TypeScript', 'Static',
] as const

const trustItems = ['records', 'commands', 'limits', 'roles'] as const
const motionEase = [0.22, 1, 0.36, 1] as const

function BrandMark({ className }: { className?: string }) {
  return <img src="/runara-icon.png" alt="" className={cn('block size-9 object-contain', className)} />
}

export default function Landing() {
  const { t, language } = useTranslation()
  const { token, user } = useAuthStore()
  const reduceMotion = useReducedMotion()
  const isAdmin = user?.role === 'superadmin' || user?.role === 'admin'
  const appPath = token ? (isAdmin ? '/admin/dashboard' : '/dashboard') : '/login'

  useEffect(() => {
    document.documentElement.lang = language
  }, [language])

  return (
    <MotionConfig reducedMotion="user">
      <div className="runara-landing h-full overflow-x-clip overflow-y-auto bg-background font-sans text-foreground antialiased">
        <a href="#main-content" className="sr-only z-[100] min-h-11 bg-primary px-4 py-3 font-semibold text-primary-foreground focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:outline-none focus:ring-2 focus:ring-ring">
          {t('landing.skipToContent')}
        </a>

        <header className="border-b border-border bg-background">
          <nav className="mx-auto flex min-h-20 max-w-[90rem] items-center justify-between gap-4 px-4 sm:px-6 lg:px-10" aria-label={t('landing.primaryNavigation')}>
            <div className="flex min-w-0 items-center gap-8">
              <Link to="/" className="flex min-h-11 shrink-0 items-center gap-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                <BrandMark />
                <span className="text-base font-extrabold tracking-[-0.03em]">Runara</span>
              </Link>
              <div className="hidden items-center gap-5 md:flex">
                {navLinks.map(([href, label]) => (
                  <a key={href} href={href} className="flex min-h-11 items-center border-b border-transparent text-sm font-semibold text-muted-foreground transition-colors hover:border-primary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                    {t(label)}
                  </a>
                ))}
              </div>
            </div>
            <div className="runara-surface flex shrink-0 items-center gap-2">
              <LanguageSwitcher className="size-11 rounded-sm border" forceDark />
              <Button nativeButton={false} size="sm" className="min-h-11 min-w-11 px-3 font-bold sm:px-4" render={<Link to={appPath} />}>
                <span className="hidden sm:inline">{token ? t('landing.dashboard') : t('landing.signIn')}</span>
                <ArrowRight className="h-4 w-4 shrink-0" aria-hidden="true" />
                <span className="sr-only sm:hidden">{token ? t('landing.dashboard') : t('landing.signIn')}</span>
              </Button>
            </div>
          </nav>
        </header>

        <main id="main-content">
          <section className="border-b border-border px-4 py-16 sm:px-6 sm:py-24 lg:px-10 lg:py-28">
            <motion.div
              className="mx-auto max-w-[90rem]"
              initial={reduceMotion ? false : { opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: reduceMotion ? 0 : 0.5, ease: motionEase }}
            >
              <div className="grid grid-cols-1 gap-10 lg:grid-cols-12 lg:gap-x-6">
                <div className="lg:col-span-9 lg:col-start-4">
                  <h1 className="max-w-6xl text-balance text-[clamp(3.25rem,8vw,7rem)] font-extrabold leading-[0.92] tracking-[-0.055em]">{t('landing.hero.title')}</h1>
                  <div className="mt-10 grid gap-8 border-t border-border pt-8 sm:grid-cols-2 lg:ml-[16.666%]">
                    <p className="max-w-[65ch] text-pretty text-lg font-medium leading-8 text-muted-foreground">{t('landing.hero.description')}</p>
                    <div className="flex flex-wrap items-start gap-4 sm:justify-end">
                      <Button nativeButton={false} size="lg" className="runara-press min-h-12 rounded-sm border border-primary-foreground px-5 font-bold" render={<Link to={appPath} />}>
                        {token ? t('landing.hero.ctaDashboard') : t('landing.hero.cta')}
                        <ArrowRight className="h-4 w-4 shrink-0" aria-hidden="true" />
                      </Button>
                      <a href="#workflow" className="flex min-h-12 items-center border-b border-foreground px-1 text-sm font-bold transition-colors hover:border-primary hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                        {t('landing.hero.secondaryCta')}
                      </a>
                    </div>
                  </div>
                </div>
              </div>

              <aside className="mt-14 grid gap-8 border border-border bg-card p-5 sm:p-7 lg:ml-[25%] lg:grid-cols-[minmax(14rem,0.7fr)_1.3fr]" aria-labelledby="stack-panel-title">
                <div>
                  <p className="text-xs font-semibold text-accent">{t('landing.stack.eyebrow')}</p>
                  <h2 id="stack-panel-title" className="mt-3 text-2xl font-bold tracking-[-0.035em] sm:text-3xl">{t('landing.stack.title')}</h2>
                  <p className="mt-4 max-w-[50ch] text-sm leading-6 text-muted-foreground">{t('landing.stack.description')}</p>
                </div>
                <p className="self-center text-sm font-medium leading-7 text-muted-foreground">{supportedStacks.join(' · ')}</p>
              </aside>
            </motion.div>
          </section>

          <section id="workflow" className="scroll-mt-4 border-b border-border px-4 py-16 sm:px-6 sm:py-24 lg:px-10 lg:py-28">
            <div className="mx-auto max-w-[90rem]">
              <div className="max-w-4xl">
                <h2 id="release-route-title" className="text-balance text-4xl font-bold leading-[0.95] tracking-[-0.045em] sm:text-6xl">{t('landing.workflow.title')}</h2>
                <p className="mt-6 max-w-[65ch] text-base font-medium leading-7 text-muted-foreground">{t('landing.workflow.description')}</p>
              </div>

              <dl className="mt-14 border-t border-border lg:mt-20">
                {releaseChecks.map((check) => (
                  <div key={check} className="grid gap-2 border-b border-border py-6 sm:grid-cols-[minmax(10rem,0.6fr)_1.4fr] sm:gap-8">
                    <dt className="text-lg font-bold tracking-[-0.02em]">{t(`landing.workflow.checks.${check}.title`)}</dt>
                    <dd className="max-w-[60ch] text-sm leading-6 text-muted-foreground">{t(`landing.workflow.checks.${check}.description`)}</dd>
                  </div>
                ))}
              </dl>

              <p className="mt-8 max-w-[70ch] text-sm leading-7 text-muted-foreground">{t('landing.workflow.outcome')}</p>
            </div>
          </section>

          <section id="features" className="scroll-mt-4 border-b border-border px-4 py-16 sm:px-6 sm:py-24 lg:px-10 lg:py-28">
            <div className="mx-auto max-w-[90rem]">
              <div className="max-w-4xl">
                <h2 className="text-balance text-4xl font-bold leading-[0.95] tracking-[-0.045em] sm:text-6xl">{t('landing.features.title')}</h2>
                <p className="mt-6 max-w-[65ch] text-base font-medium leading-7 text-muted-foreground">{t('landing.features.description')}</p>
              </div>

              <div className="mt-14 grid gap-6 lg:mt-20">
                {capabilityGroups.map(({ key, items }) => (
                  <article key={key} className="border border-border bg-card">
                    <div className="grid gap-8 p-6 sm:p-8 lg:grid-cols-12 lg:p-10">
                      <div className="lg:col-span-5">
                        <span className="text-sm font-semibold text-primary">{t(`landing.features.groups.${key}.label`)}</span>
                        <h3 className="mt-4 text-4xl font-bold leading-[0.95] tracking-[-0.045em] sm:text-5xl">{t(`landing.features.groups.${key}.title`)}</h3>
                        <p className="mt-6 max-w-xl text-base leading-7 text-muted-foreground">{t(`landing.features.groups.${key}.description`)}</p>
                      </div>
                      <dl className="border-t border-border lg:col-span-7">
                        {items.map((item) => (
                          <div key={item} className="grid gap-2 border-b border-border py-5 sm:grid-cols-[minmax(9rem,0.65fr)_1.35fr] sm:gap-6">
                            <dt className="text-sm font-bold">{t(`landing.features.groups.${key}.items.${item}.term`)}</dt>
                            <dd className="text-sm leading-6 text-muted-foreground">{t(`landing.features.groups.${key}.items.${item}.description`)}</dd>
                          </div>
                        ))}
                      </dl>
                    </div>
                  </article>
                ))}
              </div>
            </div>
          </section>

          <section id="security" className="scroll-mt-4 bg-primary text-primary-foreground">
            <div className="mx-auto max-w-[90rem] px-4 py-16 sm:px-6 sm:py-24 lg:px-10 lg:py-28">
              <div className="min-w-0">
                <h2 className="max-w-6xl break-words text-balance text-4xl font-bold leading-[0.92] tracking-[-0.05em] sm:text-7xl lg:text-8xl">{t('landing.trust.title')}</h2>
                <p className="mt-8 max-w-3xl text-lg font-semibold leading-8">{t('landing.trust.description')}</p>
              </div>
              <div className="mt-14 border-t border-primary-foreground lg:mt-20">
                {trustItems.map((item) => (
                  <article key={item} className="grid gap-3 border-b border-primary-foreground py-6 sm:grid-cols-[0.7fr_1.3fr] sm:gap-8">
                    <h3 className="text-xl font-bold">{t(`landing.trust.items.${item}.title`)}</h3>
                    <p className="max-w-3xl text-sm font-medium leading-6">{t(`landing.trust.items.${item}.description`)}</p>
                  </article>
                ))}
              </div>
              <div className="mt-10 grid gap-3 border border-primary-foreground p-5 sm:grid-cols-[0.7fr_1.3fr] sm:gap-8">
                <p className="font-bold">{t('landing.trust.transparency.title')}</p>
                <p className="text-sm font-medium leading-6">{t('landing.trust.transparency.description')}</p>
              </div>
            </div>
          </section>

          <section className="border-b border-border px-4 py-16 sm:px-6 sm:py-24 lg:px-10 lg:py-28">
            <div className="mx-auto grid min-w-0 max-w-[90rem] gap-10 lg:grid-cols-12 lg:items-end">
              <div className="min-w-0 lg:col-span-9">
                <h2 className="max-w-5xl break-words text-balance text-4xl font-bold leading-[0.92] tracking-[-0.05em] sm:text-7xl">{t('landing.final.title')}</h2>
                <p className="mt-7 max-w-[65ch] text-base leading-7 text-muted-foreground">{t('landing.final.description')}</p>
              </div>
              <div className="lg:col-span-3 lg:flex lg:justify-end">
                <Button nativeButton={false} size="lg" className="min-h-12 px-5 font-bold" render={<Link to={appPath} />}>
                  {token ? t('landing.final.dashboard') : t('landing.final.cta')}
                  <ArrowRight className="h-4 w-4 shrink-0" aria-hidden="true" />
                </Button>
              </div>
            </div>
          </section>
        </main>

        <footer className="px-4 py-8 sm:px-6 lg:px-10">
          <div className="mx-auto flex max-w-[90rem] flex-col gap-6 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-center gap-3">
              <BrandMark className="size-8" />
              <div>
                <p className="text-sm font-bold">Runara</p>
                <p className="text-xs text-muted-foreground">{t('landing.footer.tagline')}</p>
              </div>
            </div>
            <p className="text-xs text-muted-foreground">{t('landing.footer.copyright', { year: new Date().getFullYear() })}</p>
          </div>
        </footer>
      </div>
    </MotionConfig>
  )
}
