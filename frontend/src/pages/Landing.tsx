import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import useAuthStore from '../stores/authStore'
import useTranslation from '../lib/useTranslation'
import { LanguageSwitcher } from '../components/LanguageSwitcher'
import {
  Rocket, Database, Globe, RefreshCw, Terminal, ArrowRight, CheckCircle2, Ship, Layers, Zap, Shield, Cpu, ArrowUpRight, Sun, Moon
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

export default function Landing() {
  const { t } = useTranslation()
  const { token, user } = useAuthStore()
  const [activeStep, setActiveStep] = useState(0)

  const featureList = [
    { icon: Zap, title: t('landing.featureList.atomic.title'), desc: t('landing.featureList.atomic.desc') },
    { icon: Database, title: t('landing.featureList.db.title'), desc: t('landing.featureList.db.desc') },
    { icon: Globe, title: t('landing.featureList.edge.title'), desc: t('landing.featureList.edge.desc') },
    { icon: Layers, title: t('landing.featureList.php.title'), desc: t('landing.featureList.php.desc') },
    { icon: Terminal, title: t('landing.featureList.access.title'), desc: t('landing.featureList.access.desc') },
    { icon: Shield, title: t('landing.featureList.secure.title'), desc: t('landing.featureList.secure.desc') },
  ]

  const rawWorkflowSteps = t('landing.workflowSteps')
  const workflowSteps = Array.isArray(rawWorkflowSteps) ? rawWorkflowSteps : []

  const [isDark, setIsDark] = useState(() => {
    if (typeof window !== 'undefined') {
      const savedTheme = localStorage.getItem('theme')
      if (savedTheme) {
        return savedTheme === 'dark'
      }
      return window.matchMedia('(prefers-color-scheme: dark)').matches
    }
    return true
  })

  useEffect(() => {
    if (isDark) {
      document.documentElement.classList.add('dark')
      localStorage.setItem('theme', 'dark')
    } else {
      document.documentElement.classList.remove('dark')
      localStorage.setItem('theme', 'light')
    }
  }, [isDark])

  const isAdmin = user?.role === 'superadmin' || user?.role === 'admin'
  const dashboardPath = isAdmin ? '/admin/dashboard' : '/dashboard'

  useEffect(() => {
    const interval = setInterval(() => {
      setActiveStep((prev) => (prev + 1) % workflowSteps.length)
    }, 3000)
    return () => clearInterval(interval)
  }, [workflowSteps.length])

  return (
    <div className="min-h-screen bg-background text-foreground font-sans antialiased text-sm">
      {/* Navigation */}
      <nav className="border-b bg-background/80 backdrop-blur-md sticky top-0 z-50">
        <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
          <div className="flex items-center gap-8">
            <Link to="/" className="flex items-center gap-2 group">
              <div className="w-8 h-8 bg-primary text-primary-foreground rounded-md flex items-center justify-center font-bold tracking-tighter">
                LP
              </div>
              <span className="font-bold tracking-tight group-hover:text-primary transition-colors">Runara</span>
            </Link>

            <div className="hidden md:flex items-center gap-6">
              <a href="#features" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors uppercase tracking-widest">{t('landing.features')}</a>
              <a href="#workflow" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors uppercase tracking-widest">{t('landing.workflow')}</a>
              <a href="#infrastructure" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors uppercase tracking-widest">{t('landing.infrastructure')}</a>
            </div>
          </div>

          <div className="flex items-center gap-4">
            <LanguageSwitcher />

            <Button variant="ghost" size="icon" onClick={() => setIsDark(!isDark)}>
              {isDark ? <Sun className="w-4 h-4 transition-transform" /> : <Moon className="w-4 h-4 transition-transform" />}
            </Button>
            <Button render={<Link to={token ? dashboardPath : "/login"} />}>
                {token ? t('landing.dashboard') : t('landing.signIn')}
                <ArrowUpRight className="ml-2 w-4 h-4" />
            </Button>
          </div>
        </div>
      </nav>

      {/* Hero Section */}
      <section className="pt-32 pb-40 px-6">
        <div className="max-w-4xl mx-auto text-center">
          <Badge variant="secondary" className="mb-8 p-1.5 uppercase font-bold tracking-widest text-[10px]">
            <Zap className="w-3 h-3 mr-2 text-primary" />
            {t('landing.heroBadge')}
          </Badge>

          <h1 className="text-5xl md:text-7xl font-bold tracking-tight mb-8 whitespace-pre-line">
            {t('landing.heroTitle')}
          </h1>

          <p className="text-lg md:text-xl text-muted-foreground mb-12 max-w-2xl mx-auto">
            {t('landing.heroSubtitle')}
          </p>

          <div className="flex flex-col sm:flex-row items-center justify-center gap-4">
            <Button size="lg" render={<Link to={token ? dashboardPath : "/login"} />}>
                {token ? t('landing.returnDashboard') : t('landing.deployFirst')}
                <ArrowRight className="ml-2 w-4 h-4" />
            </Button>
            <Button size="lg" variant="outline" render={<a href="https://github.com/laravell-paas/repo" target="_blank" rel="noreferrer" />}>
                <Rocket className="mr-2 w-4 h-4" />
                {t('landing.viewSource')}
            </Button>
          </div>
        </div>

        {/* Dynamic Interface Preview */}
        <div id="workflow" className="max-w-6xl mx-auto mt-24">
          <Card className="overflow-hidden border shadow-lg">
            <div className="h-11 bg-muted flex items-center px-4 justify-between border-b">
              <div className="flex gap-2">
                <div className="w-3 h-3 rounded-full bg-border"></div>
                <div className="w-3 h-3 rounded-full bg-border"></div>
                <div className="w-3 h-3 rounded-full bg-border"></div>
              </div>
              <div className="text-xs font-mono text-muted-foreground flex items-center gap-2">
                <Terminal className="w-3 h-3" />
                {t('landing.terminal.header')}
              </div>
              <div />
            </div>

            <div className="p-8 grid grid-cols-1 md:grid-cols-2 gap-12 items-center bg-card">
              <div className="space-y-6">
                {workflowSteps.map((step, i) => (
                  <div key={i} className={`transition-opacity duration-700 ${activeStep === i ? 'opacity-100' : 'opacity-30'}`}>
                    <p className="text-[10px] font-bold text-primary mb-1 uppercase tracking-widest">Step 0{i + 1}</p>
                    <h4 className="text-lg font-bold mb-2">{step.title}</h4>
                    <div className="bg-muted border rounded-md py-2 px-4 font-mono text-xs text-muted-foreground">
                      {step.url}
                    </div>
                  </div>
                ))}
              </div>

              <div className="aspect-video rounded-lg bg-black text-green-400 p-6 font-mono text-xs overflow-hidden shadow-inner border">
                <div className="flex items-center gap-2 mb-4">
                  <span className="w-2 h-2 rounded-full bg-green-500"></span>
                  <span className="text-[10px] uppercase font-bold tracking-wider text-green-500">Live Feed</span>
                </div>
                <p className="mb-1 text-blue-400">{t('landing.terminal.init')}</p>
                <p className="mb-1 text-blue-400">{t('landing.terminal.pull')}</p>
                <p className="mb-1 text-blue-400">{t('landing.terminal.inject')}</p>
                <p className="mb-1 text-white">{t('landing.terminal.composer')}</p>
                <p className="mb-1 text-gray-400">{t('landing.terminal.installing')}</p>
                <p className="mb-1 text-yellow-500">{t('landing.terminal.sqlite')}</p>
                <p className="mb-1 text-blue-400">{t('landing.terminal.mapping')}</p>
                <p className="mt-6 text-green-500 font-bold animate-pulse">{t('landing.terminal.success')}</p>
              </div>
            </div>
          </Card>
        </div>
      </section>

      {/* Features Grid */}
      <section id="features" className="py-32 px-6 border-t bg-muted/30">
        <div className="max-w-7xl mx-auto">
          <div className="mb-16 max-w-2xl">
            <h2 className="text-3xl font-bold tracking-tight mb-4">{t('landing.builtPrecision')}</h2>
            <p className="text-muted-foreground">{t('landing.precisionDesc')}</p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
            {featureList.map((feature, i) => (
              <Card key={i} className="hover:border-primary/50 transition-colors bg-card p-2 rounded-2xl group">
                <CardContent className="p-6">
                  <div className="w-10 h-10 bg-primary/10 text-primary rounded-lg flex items-center justify-center mb-4 group-hover:scale-110 transition-transform">
                    <feature.icon className="w-5 h-5" />
                  </div>
                  <h3 className="text-base font-bold mb-2">{feature.title}</h3>
                  <p className="text-xs text-muted-foreground leading-relaxed">{feature.desc}</p>
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      </section>

      {/* Infrastructure Callout */}
      <section id="infrastructure" className="py-32 px-6 border-t">
        <div className="max-w-7xl mx-auto">
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-16 items-center">
            <div className="lg:col-span-6">
              <div className="grid grid-cols-2 gap-4">
                <Card className="p-6 border bg-card hover:border-primary/30 transition-colors">
                  <Cpu className="w-6 h-6 text-primary mb-4" />
                  <h4 className="font-bold mb-2">{t('landing.infra.cpu.title')}</h4>
                  <p className="text-xs text-muted-foreground line-clamp-3">{t('landing.infra.cpu.desc')}</p>
                </Card>
                <Card className="p-6 border bg-card translate-y-6 hover:border-primary/30 transition-colors">
                  <RefreshCw className="w-6 h-6 text-primary mb-4" />
                  <h4 className="font-bold mb-2">{t('landing.infra.healing.title')}</h4>
                  <p className="text-xs text-muted-foreground line-clamp-3">{t('landing.infra.healing.desc')}</p>
                </Card>
                <Card className="p-6 border bg-card hover:border-primary/30 transition-colors">
                  <Database className="w-6 h-6 text-primary mb-4" />
                  <h4 className="font-bold mb-2">{t('landing.infra.maria.title')}</h4>
                  <p className="text-xs text-muted-foreground line-clamp-3">{t('landing.infra.maria.desc')}</p>
                </Card>
                <Card className="p-6 border bg-card translate-y-6 hover:border-primary/30 transition-colors">
                  <Ship className="w-6 h-6 text-primary mb-4" />
                  <h4 className="font-bold mb-2">{t('landing.infra.docker.title')}</h4>
                  <p className="text-xs text-muted-foreground line-clamp-3">{t('landing.infra.docker.desc')}</p>
                </Card>
              </div>
            </div>

            <div className="lg:col-span-6 lg:pl-8 mt-10 lg:mt-0">
              <Badge variant="outline" className="mb-4 uppercase tracking-widest text-[10px] p-1.5 font-bold">System Architecture</Badge>
              <h3 className="text-3xl font-bold mb-4 tracking-tight">{t('landing.infra.title')}</h3>
              <p className="text-muted-foreground mb-8 text-lg">{t('landing.infra.desc')}</p>

              <ul className="space-y-4">
                {(Array.isArray(t('landing.infra.list')) ? t('landing.infra.list') : []).map((item: string, i: number) => (
                  <li key={i} className="flex items-center gap-3 text-sm font-medium">
                    <CheckCircle2 className="w-4 h-4 text-primary" />
                    {item}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
      </section>

      {/* Final CTA */}
      <section className="py-32 px-6 border-t bg-muted/30">
        <div className="max-w-4xl mx-auto text-center">
          <h2 className="text-3xl md:text-5xl font-bold tracking-tight mb-6">{t('landing.stopConfiguring')}</h2>
          <p className="text-muted-foreground text-lg mb-10 max-w-xl mx-auto">{t('landing.finalCTA')}</p>

          <Button size="lg" render={<Link to={token ? dashboardPath : "/login"} />}>
              {token ? t('landing.goDashboard') : t('landing.readyDeploy')}
              <ArrowRight className="ml-2 w-4 h-4" />
          </Button>
        </div>
      </section>

      {/* Footer */}
      <footer className="py-12 px-6 border-t bg-background">
        <div className="max-w-7xl mx-auto flex flex-col md:flex-row justify-between items-center gap-6">
          <div className="flex items-center gap-3">
            <div className="w-6 h-6 bg-primary text-primary-foreground rounded flex items-center justify-center text-xs font-bold font-mono">R</div>
            <span className="text-sm font-semibold tracking-tight">Runara {t('common.logoSub')}</span>
          </div>
          <p className="text-[10px] text-muted-foreground uppercase font-bold tracking-widest">
            &copy; {new Date().getFullYear()} Advanced Analytics Cluster.
          </p>
          <div className="flex items-center gap-4 text-muted-foreground">
            <Rocket className="w-4 h-4" />
          </div>
        </div>
      </footer>
    </div>
  )
}
