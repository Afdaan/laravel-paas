import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import useAuthStore from '../stores/authStore'
import {
  Rocket, Database, Globe, Compass, RefreshCw, Terminal, ArrowRight, CheckCircle2, ChevronRight, Ship, Layers, Zap, Shield, Cpu, ArrowUpRight, Sun, Moon
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

const features = [
  { icon: Zap, title: 'Atomic Deployments', desc: 'Zero-downtime pushes from GitHub. Your application state is preserved while your new code goes live.' },
  { icon: Database, title: 'Autoprovisioned DB', desc: 'Each project receives a dedicated MariaDB instance, pre-configured with the correct environment variables.' },
  { icon: Globe, title: 'Edge Routing', desc: 'Automatic SSL and subdomain mapping on our high-performance Nginx edge cluster.' },
  { icon: Layers, title: 'PHP Multiverse', desc: 'Native support for PHP 8.0 through 8.4. Switch runtimes instantly via your project control panel.' },
  { icon: Terminal, title: 'Runtime Access', desc: 'Execute Artisan commands and monitor live system logs through a secure web-based terminal.' },
  { icon: Shield, title: 'Secure Isolation', desc: 'Every student project runs in a dedicated Docker container, ensuring complete environment sandboxing.' },
]

const steps = [
  { title: 'Connect repository', url: 'github.com/your-username/repo' },
  { title: 'Select PHP Version', url: '8.4 (Latest)' },
  { title: 'Connect to Server', url: 'system.paas.io/live' },
]

export default function Landing() {
  const { token, user } = useAuthStore()
  const [activeStep, setActiveStep] = useState(0)

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
      setActiveStep((prev) => (prev + 1) % steps.length)
    }, 3000)
    return () => clearInterval(interval)
  }, [])

  return (
    <div className="min-h-screen bg-background text-foreground font-sans antialiased">
      {/* Navigation */}
      <nav className="border-b bg-background/80 backdrop-blur-md sticky top-0 z-50">
        <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
          <div className="flex items-center gap-8">
            <Link to="/" className="flex items-center gap-2 group">
              <div className="w-8 h-8 bg-primary text-primary-foreground rounded-md flex items-center justify-center font-bold tracking-tighter">
                LP
              </div>
              <span className="font-bold tracking-tight group-hover:text-primary transition-colors">Laravel PaaS</span>
            </Link>

            <div className="hidden md:flex items-center gap-6">
              <a href="#features" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors">Features</a>
              <a href="#workflow" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors">Workflow</a>
              <a href="#infrastructure" className="text-sm font-medium text-muted-foreground hover:text-foreground transition-colors">Infrastructure</a>
            </div>
          </div>

          <div className="flex items-center gap-4">
            <Button variant="ghost" size="icon" onClick={() => setIsDark(!isDark)}>
              {isDark ? <Sun className="w-4 h-4 transition-transform" /> : <Moon className="w-4 h-4 transition-transform" />}
            </Button>
            <Button render={<Link to={token ? dashboardPath : "/login"} />}>
                {token ? 'Dashboard' : 'Sign In'}
                <ArrowUpRight className="ml-2 w-4 h-4" />
            </Button>
          </div>
        </div>
      </nav>

      {/* Hero Section */}
      <section className="pt-32 pb-40 px-6">
        <div className="max-w-4xl mx-auto text-center">
          <Badge variant="secondary" className="mb-8">
            <Zap className="w-3 h-3 mr-2" />
            Next Gen Hosting for Students
          </Badge>

          <h1 className="text-5xl md:text-7xl font-bold tracking-tight mb-8">
            Laravel deployment, <br />
            <span className="text-muted-foreground">reimagined</span> for students.
          </h1>

          <p className="text-lg md:text-xl text-muted-foreground mb-12 max-w-2xl mx-auto">
            A minimalist cloud platform that automates your repository orchestration, database provisioning, and SSL routing. Focus on code, not infrastructure.
          </p>

          <div className="flex flex-col sm:flex-row items-center justify-center gap-4">
            <Button size="lg" render={<Link to={token ? dashboardPath : "/login"} />}>
                {token ? 'Go to Dashboard' : 'Deploy your first project'}
                <ArrowRight className="ml-2 w-4 h-4" />
            </Button>
            <Button size="lg" variant="outline" render={<a href="https://github.com/laravell-paas/repo" target="_blank" rel="noreferrer" />}>
                <Rocket className="mr-2 w-4 h-4" />
                View Source Code
            </Button>
          </div>
        </div>

        {/* Dynamic Interface Preview */}
        <div className="max-w-6xl mx-auto mt-24">
          <Card className="overflow-hidden border shadow-lg">
            <div className="h-11 bg-muted flex items-center px-4 justify-between border-b">
              <div className="flex gap-2">
                <div className="w-3 h-3 rounded-full bg-border"></div>
                <div className="w-3 h-3 rounded-full bg-border"></div>
                <div className="w-3 h-3 rounded-full bg-border"></div>
              </div>
              <div className="text-xs font-mono text-muted-foreground flex items-center gap-2">
                <Terminal className="w-3 h-3" />
                bash — project-uplink.sh — 80×24
              </div>
              <div />
            </div>

            <div className="p-8 grid grid-cols-1 md:grid-cols-2 gap-12 items-center bg-card">
              <div className="space-y-6">
                {steps.map((step, i) => (
                  <div key={i} className={`transition-opacity duration-700 ${activeStep === i ? 'opacity-100' : 'opacity-30'}`}>
                    <p className="text-xs font-semibold text-primary mb-1 uppercase tracking-wider">Step 0{i + 1}</p>
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
                <p className="mb-1 text-blue-400">[INFO] Initializing architecture...</p>
                <p className="mb-1 text-blue-400">[INFO] Pulling source from origin/main...</p>
                <p className="mb-1 text-blue-400">[INFO] Injecting PHP 8.3 container...</p>
                <p className="mb-1 text-white">&gt; composer install --no-dev</p>
                <p className="mb-1 text-gray-400">Installing dependencies (92%)...</p>
                <p className="mb-1 text-yellow-500">[WARN] SQLite fallback enabled</p>
                <p className="mb-1 text-blue-400">[INFO] Mapping subdomain: your-app.paas.io</p>
                <p className="mt-6 text-green-500 font-bold animate-pulse">✓ Deployment Successful</p>
              </div>
            </div>
          </Card>
        </div>
      </section>

      {/* Features Grid */}
      <section id="features" className="py-32 px-6 border-t bg-muted/30">
        <div className="max-w-7xl mx-auto">
          <div className="mb-16 max-w-2xl">
            <h2 className="text-3xl font-bold tracking-tight mb-4">Built for precision.</h2>
            <p className="text-muted-foreground">Every technical hurdle between your repository and a live URL has been automated by our engineering core.</p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
            {features.map((feature, i) => (
              <Card key={i} className="hover:border-primary/50 transition-colors bg-card">
                <CardContent className="p-6">
                  <div className="w-10 h-10 bg-primary/10 text-primary rounded-lg flex items-center justify-center mb-4">
                    <feature.icon className="w-5 h-5" />
                  </div>
                  <h3 className="text-base font-bold mb-2">{feature.title}</h3>
                  <p className="text-sm text-muted-foreground">{feature.desc}</p>
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
                <Card className="p-6 border bg-card">
                  <Cpu className="w-6 h-6 text-primary mb-4" />
                  <h4 className="font-bold mb-2">Dedicated CPU</h4>
                  <p className="text-sm text-muted-foreground line-clamp-3">Guaranteed cycles for your PHP processes.</p>
                </Card>
                <Card className="p-6 border bg-card translate-y-6">
                  <RefreshCw className="w-6 h-6 text-primary mb-4" />
                  <h4 className="font-bold mb-2">Auto-Healing</h4>
                  <p className="text-sm text-muted-foreground line-clamp-3">System automatically restarts crashed containers.</p>
                </Card>
                <Card className="p-6 border bg-card">
                  <Database className="w-6 h-6 text-primary mb-4" />
                  <h4 className="font-bold mb-2">MariaDB Stack</h4>
                  <p className="text-sm text-muted-foreground line-clamp-3">Native MySQL-compatible storage backend.</p>
                </Card>
                <Card className="p-6 border bg-card translate-y-6">
                  <Ship className="w-6 h-6 text-primary mb-4" />
                  <h4 className="font-bold mb-2">Docker Core</h4>
                  <p className="text-sm text-muted-foreground line-clamp-3">Lightweight isolated containerization.</p>
                </Card>
              </div>
            </div>

            <div className="lg:col-span-6 lg:pl-8 mt-10 lg:mt-0">
              <Badge variant="outline" className="mb-4">System Architecture</Badge>
              <h3 className="text-3xl font-bold mb-4 tracking-tight">Enterprise grade <br />infrastructure.</h3>
              <p className="text-muted-foreground mb-8 text-lg">We operate our own private cloud cluster, optimized specifically for the PHP and Laravel runtime lifecycle.</p>

              <ul className="space-y-4">
                {[
                  'High Availability Edge Cluster',
                  'Isolated MariaDB Instances',
                  'NVMe SSD Storage Backend',
                  'Automated SSL Termination'
                ].map((item, i) => (
                  <li key={i} className="flex items-center gap-3 text-sm">
                    <CheckCircle2 className="w-5 h-5 text-primary" />
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
          <h2 className="text-3xl md:text-5xl font-bold tracking-tight mb-6">Stop configuring. Start building.</h2>
          <p className="text-muted-foreground text-lg mb-10 max-w-xl mx-auto">Join hundreds of students currently shipping their Laravel projects on the fastest platform in the cluster.</p>

          <Button size="lg" render={<Link to={token ? dashboardPath : "/login"} />}>
              {token ? 'Return to Dashboard' : 'Ready to deploy?'}
              <ArrowRight className="ml-2 w-4 h-4" />
          </Button>
        </div>
      </section>

      {/* Footer */}
      <footer className="py-12 px-6 border-t bg-background">
        <div className="max-w-7xl mx-auto flex flex-col md:flex-row justify-between items-center gap-6">
          <div className="flex items-center gap-3">
            <div className="w-6 h-6 bg-primary text-primary-foreground rounded flex items-center justify-center text-xs font-bold">LP</div>
            <span className="text-sm font-semibold">Laravel PaaS Core</span>
          </div>
          <p className="text-xs text-muted-foreground">
            &copy; {new Date().getFullYear()} Advanced Analytics Cluster.
          </p>
          <div className="flex items-center gap-4">
            <a href="#" className="text-muted-foreground hover:text-foreground transition-colors"><Rocket className="w-5 h-5" /></a>
          </div>
        </div>
      </footer>
    </div>
  )
}
