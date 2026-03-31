// ===========================================
// Landing Page (Modern Minimalist Refactor)
// ===========================================
// Human-centric design focused on clarity
// ===========================================

import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import useAuthStore from '../stores/authStore'
import { 
  Rocket, 
  Database, 
  Globe, 
  Compass, 
  RefreshCw, 
  Terminal, 
  ArrowRight, 
  CheckCircle2, 
  ChevronRight, 
  Github,
  Ship,
  Layers,
  Zap,
  Shield,
  Cpu,
  ArrowUpRight
} from 'lucide-react'

const features = [
  {
    icon: Zap,
    title: 'Atomic Deployments',
    desc: 'Zero-downtime pushes from GitHub. Your application state is preserved while your new code goes live.',
  },
  {
    icon: Database,
    title: 'Autoprovisioned DB',
    desc: 'Each project receives a dedicated MariaDB instance, pre-configured with the correct environment variables.',
  },
  {
    icon: Globe,
    title: 'Edge Routing',
    desc: 'Automatic SSL and subdomain mapping on our high-performance Nginx edge cluster.',
  },
  {
    icon: Layers,
    title: 'PHP Multiverse',
    desc: 'Native support for PHP 8.0 through 8.4. Switch runtimes instantly via your project control panel.',
  },
  {
    icon: Terminal,
    title: 'Runtime Access',
    desc: 'Execute Artisan commands and monitor live system logs through a secure web-based terminal.',
  },
  {
    icon: Shield,
    title: 'Secure Isolation',
    desc: 'Every student project runs in a dedicated Docker container, ensuring complete environment sandboxing.',
  },
]

const steps = [
  { title: 'Connect repository', url: 'github.com/your-username/repo' },
  { title: 'Select PHP Version', url: '8.4 (Latest)' },
  { title: 'Connect to Server', url: 'system.paas.io/live' },
]

export default function Landing() {
  const { token, user } = useAuthStore()
  const [activeStep, setActiveStep] = useState(0)

  const isAdmin = user?.role === 'superadmin' || user?.role === 'admin'
  const dashboardPath = isAdmin ? '/admin/dashboard' : '/dashboard'

  useEffect(() => {
    const interval = setInterval(() => {
      setActiveStep((prev) => (prev + 1) % steps.length)
    }, 3000)
    return () => clearInterval(interval)
  }, [])

  return (
    <div className="min-h-screen bg-white dark:bg-slate-950 text-[#f8fafc] selection:bg-indigo-500/30 font-sans antialiased">
      {/* Subtle Grid Background */}
      <div className="fixed inset-0 pointer-events-none opacity-[0.03] z-0 bg-[url('https://grainy-gradients.vercel.app/noise.svg')]"></div>
      <div className="fixed inset-0 pointer-events-none z-0 overflow-hidden">
         <div className="absolute top-0 left-1/2 -translate-x-1/2 w-full h-[500px] bg-gradient-to-b from-indigo-500/5 to-transparent"></div>
      </div>

      {/* Navigation */}
      <nav className="relative z-50 border-b border-slate-200 dark:border-slate-200 dark:border-white/5 bg-white dark:bg-slate-950/80 backdrop-blur-xl sticky top-0">
        <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
          <div className="flex items-center gap-8">
            <Link to="/" className="flex items-center gap-2.5 group">
              <div className="w-8 h-8 bg-white text-black rounded-lg flex items-center justify-center font-black tracking-tighter">
                LP
              </div>
              <span className="font-bold tracking-tight text-slate-900 dark:text-white group-hover:text-indigo-400 transition-colors">Laravel PaaS</span>
            </Link>
            
            <div className="hidden md:flex items-center gap-6">
               <a href="#features" className="text-sm font-medium text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white transition-colors">Features</a>
               <a href="#workflow" className="text-sm font-medium text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white transition-colors">Workflow</a>
               <a href="#infrastructure" className="text-sm font-medium text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white transition-colors">Infrastructure</a>
            </div>
          </div>

          <div className="flex items-center gap-4">
            <Link
              to={token ? dashboardPath : "/login"}
              className="text-sm font-bold text-slate-900 dark:text-white bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 px-5 py-2 rounded-xl hover:bg-slate-200 dark:bg-white/10 transition-all flex items-center gap-2"
            >
              {token ? 'Dashboard' : 'Sign In'}
              <ArrowUpRight className="w-3.5 h-3.5" />
            </Link>
          </div>
        </div>
      </nav>

      {/* Hero Section */}
      <section className="relative z-10 pt-32 pb-40 px-6">
        <div className="max-w-4xl mx-auto text-center">
          <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-indigo-500/10 border border-indigo-500/20 text-indigo-400 text-xs font-bold uppercase tracking-wider mb-10">
             <Zap className="w-3 h-3" />
             Next Gen Hosting for Students
          </div>
          
          <h1 className="text-5xl md:text-7xl font-bold tracking-tight text-slate-900 dark:text-white mb-8 leading-[1.05]">
            Laravel deployment, <br />
            <span className="text-slate-600 dark:text-slate-400 italic font-serif group">reimagined</span> for students.
          </h1>
          
          <p className="text-lg md:text-xl text-slate-600 dark:text-slate-400 mb-12 max-w-2xl mx-auto leading-relaxed">
            A minimalist cloud platform that automates your repository orchestration, database provisioning, and SSL routing. Focus on code, not infrastructure.
          </p>

          <div className="flex flex-col sm:flex-row items-center justify-center gap-4">
             <Link
                to={token ? dashboardPath : "/login"}
                className="w-full sm:w-auto px-8 py-4 bg-white text-black font-bold rounded-xl hover:bg-slate-200 transition-all flex items-center justify-center gap-3 shadow-2xl shadow-white/5"
             >
                {token ? 'Go to Dashboard' : 'Deploy your first project'}
                <ArrowRight className="w-4 h-4" />
             </Link>
             <button className="w-full sm:w-auto px-8 py-4 bg-slate-100 dark:bg-white/5 text-slate-900 dark:text-white border border-slate-300 dark:border-white/10 font-bold rounded-xl hover:bg-slate-200 dark:bg-white/10 transition-all flex items-center justify-center gap-3">
                <Github className="w-4 h-4" />
                View Source Code
             </button>
          </div>
        </div>

        {/* Dynamic Interface Preview */}
        <div className="max-w-6xl mx-auto mt-32 relative">
           
           
           <div className="bg-[#0f0f13] border border-slate-200 dark:border-white/5 rounded-2xl overflow-hidden shadow-2xl shadow-black">
              {/* Terminal Ribbon */}
              <div className="h-11 bg-[#16161c] border-b border-slate-200 dark:border-white/5 flex items-center px-5 justify-between">
                 <div className="flex gap-2">
                    <div className="w-3 h-3 rounded-full bg-slate-200 dark:bg-white/10"></div>
                    <div className="w-3 h-3 rounded-full bg-slate-200 dark:bg-white/10"></div>
                    <div className="w-3 h-3 rounded-full bg-slate-200 dark:bg-white/10"></div>
                 </div>
                 <div className="text-[10px] font-mono text-slate-600 dark:text-slate-400 flex items-center gap-2">
                    <Terminal className="w-3 h-3" />
                    bash — project-uplink.sh — 80×24
                 </div>
                 <div></div>
              </div>
              
              <div className="p-10 grid grid-cols-1 md:grid-cols-2 gap-16 items-center">
                 <div className="space-y-8">
                    {steps.map((step, i) => (
                       <div key={i} className={`transition-all duration-700 ${activeStep === i ? 'opacity-100 translate-x-4' : 'opacity-30'}`}>
                          <p className="text-[10px] font-black uppercase tracking-[0.2em] text-indigo-400 mb-2">Step 0{i+1}</p>
                          <h4 className="text-xl font-bold text-slate-900 dark:text-white mb-2">{step.title}</h4>
                          <div className="bg-black/40 border border-slate-200 dark:border-white/5 rounded-lg py-2 px-4 font-mono text-xs text-slate-600 dark:text-slate-400">
                             {step.url}
                          </div>
                       </div>
                    ))}
                 </div>
                 
                 <div className="relative aspect-video rounded-xl bg-black/60 border border-slate-200 dark:border-white/5 overflow-hidden p-6 font-mono text-[11px] leading-relaxed text-slate-600 dark:text-slate-400">
                    <div className="flex items-center gap-2 mb-6">
                       <div className="w-2 h-2 rounded-full bg-emerald-500"></div>
                       <span className="text-emerald-500 text-[10px] font-black uppercase">Live System Feed</span>
                    </div>
                    <p className="mb-2"><span className="text-indigo-400">INFO</span> [2024-10-12 14:02] Initializing architecture...</p>
                    <p className="mb-2"><span className="text-indigo-400">INFO</span> [2024-10-12 14:02] Pulling source from origin/main...</p>
                    <p className="mb-2"><span className="text-indigo-400">INFO</span> [2024-10-12 14:03] Injecting PHP 8.3 container...</p>
                    <p className="mb-2 text-slate-900 dark:text-white font-bold">&gt; composer install --no-dev</p>
                    <p className="mb-2 text-slate-600">Installing dependencies (92%)...</p>
                    <p className="mb-2"><span className="text-amber-400">WARN</span> SQLite fallback enabled</p>
                    <p className="mb-2"><span className="text-indigo-400">INFO</span> Mapping subdomain: your-app.paas.io</p>
                    <p className="mt-8 text-emerald-400 font-bold tracking-widest uppercase animate-pulse">✓ Deployment Successful</p>
                 </div>
              </div>
           </div>
        </div>
      </section>

      {/* Features Grid */}
      <section id="features" className="py-40 px-6 border-t border-slate-200 dark:border-white/5 bg-[#070709]">
        <div className="max-w-7xl mx-auto">
          <div className="mb-24">
            <h2 className="text-4xl font-bold text-slate-900 dark:text-white tracking-tight mb-6">Built for precision.</h2>
            <p className="text-slate-600 dark:text-slate-400 max-w-xl">Every technical hurdle between your repository and a live URL has been automated by our engineering core.</p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-y-16 gap-x-12">
            {features.map((feature, i) => (
              <div key={i} className="group cursor-default">
                <div className="w-12 h-12 bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 rounded-xl flex items-center justify-center mb-6 group-hover:bg-white group-hover:text-black transition-all">
                  <feature.icon className="w-5 h-5 transition-transform group-hover:scale-110" />
                </div>
                <h3 className="text-lg font-bold text-slate-900 dark:text-white mb-3 tracking-tight">{feature.title}</h3>
                <p className="text-slate-600 dark:text-slate-400 text-sm leading-relaxed">{feature.desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Infrastructure Callout */}
      <section id="infrastructure" className="py-40 px-6 overflow-hidden">
         <div className="max-w-7xl mx-auto">
            <div className="grid grid-cols-1 lg:grid-cols-12 gap-20 items-center">
               <div className="lg:col-span-7">
                  <div className="grid grid-cols-2 gap-4">
                     <div className="p-8 rounded-2xl bg-[#0f0f13] border border-slate-200 dark:border-white/5 space-y-4">
                        <Cpu className="w-8 h-8 text-indigo-400" />
                        <h4 className="font-bold text-slate-900 dark:text-white">Dedicated CPU</h4>
                        <p className="text-xs text-slate-600 dark:text-slate-400 leading-relaxed">Guaranteed cycles for your PHP-FPM processes without noisy neighbors.</p>
                     </div>
                     <div className="p-8 rounded-2xl bg-[#0f0f13] border border-slate-200 dark:border-white/5 space-y-4 translate-y-8">
                        <RefreshCw className="w-8 h-8 text-emerald-400" />
                        <h4 className="font-bold text-slate-900 dark:text-white">Auto-Healing</h4>
                        <p className="text-xs text-slate-600 dark:text-slate-400 leading-relaxed">System monitoring automatically restarts crashed project containers.</p>
                     </div>
                     <div className="p-8 rounded-2xl bg-[#0f0f13] border border-slate-200 dark:border-white/5 space-y-4">
                        <Database className="w-8 h-8 text-rose-400" />
                        <h4 className="font-bold text-slate-900 dark:text-white">MariaDB Stack</h4>
                        <p className="text-xs text-slate-600 dark:text-slate-400 leading-relaxed">Native MySQL-compatible storage with automatic backup scheduling.</p>
                     </div>
                     <div className="p-8 rounded-2xl bg-[#0f0f13] border border-slate-200 dark:border-white/5 space-y-4 translate-y-8">
                        <Ship className="w-8 h-8 text-amber-400" />
                        <h4 className="font-bold text-slate-900 dark:text-white">Docker Core</h4>
                        <p className="text-xs text-slate-600 dark:text-slate-400 leading-relaxed">Lightweight containerization for efficient resource management.</p>
                     </div>
                  </div>
               </div>
               
               <div className="lg:col-span-5">
                  <h2 className="text-[10px] font-black tracking-[0.4em] text-indigo-400 uppercase mb-4">System Architecture</h2>
                  <h3 className="text-4xl font-bold text-slate-900 dark:text-white mb-6 tracking-tight">Enterprise grade <br />infrastructure.</h3>
                  <p className="text-slate-600 dark:text-slate-400 leading-relaxed mb-10">We operate our own private cloud cluster, optimized specifically for the PHP and Laravel runtime lifecycle.</p>
                  
                  <ul className="space-y-4">
                     {[
                        'High Availability Edge Cluster',
                        'Isolated MariaDB Instances',
                        'NVMe SSD Storage Backend',
                        'Automated SSL Termination'
                     ].map((item, i) => (
                        <li key={i} className="flex items-center gap-3 text-sm text-slate-700 dark:text-slate-300">
                           <CheckCircle2 className="w-4 h-4 text-indigo-500" />
                           {item}
                        </li>
                     ))}
                  </ul>
               </div>
            </div>
         </div>
      </section>

      {/* Final CTA */}
      <section className="py-40 px-6 border-t border-slate-200 dark:border-white/5">
        <div className="max-w-4xl mx-auto text-center">
          <h2 className="text-4xl md:text-5xl font-bold text-slate-900 dark:text-white mb-8 tracking-tight">Stop configuring. Start building.</h2>
          <p className="text-slate-600 dark:text-slate-400 text-lg mb-12 max-w-xl mx-auto">Join hundreds of students currently shipping their Laravel projects on the fastest platform in the cluster.</p>
          
          <Link
            to={token ? dashboardPath : "/login"}
            className="inline-flex items-center gap-3 px-10 py-5 bg-white text-black font-bold rounded-xl hover:bg-slate-200 transition-all text-lg shadow-2xl shadow-white/5"
          >
            {token ? 'Return to Dashboard' : 'Ready to deploy?'}
            <ArrowRight className="w-5 h-5" />
          </Link>
        </div>
      </section>

      {/* Footer */}
      <footer className="py-20 px-6 border-t border-slate-200 dark:border-white/5 bg-[#070709]">
        <div className="max-w-7xl mx-auto flex flex-col md:flex-row justify-between items-center gap-8">
          <div className="flex items-center gap-4">
             <div className="w-6 h-6 bg-slate-200 dark:bg-white/10 rounded flex items-center justify-center text-[10px] font-bold text-slate-900 dark:text-white">LP</div>
             <span className="text-sm font-bold text-slate-600 dark:text-slate-400">Laravel PaaS Core</span>
          </div>
          
          <p className="text-xs text-slate-600 font-medium">
             &copy; {new Date().getFullYear()} Advanced Analytics Cluster. Designed for high-performance students.
          </p>

          <div className="flex items-center gap-6">
             <a href="#" className="text-slate-600 hover:text-slate-900 dark:text-white transition-colors"><Github size={18} /></a>
          </div>
        </div>
      </footer>
    </div>
  )
}
