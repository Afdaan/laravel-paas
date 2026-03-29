// ===========================================
// Landing Page
// ===========================================
// Public overview page with login CTA
// ===========================================

import { useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import useAuthStore from '../stores/authStore'
import { Rocket, Database, Globe, Compass, RefreshCw, Terminal, ArrowRight, CheckCircle2, ChevronRight, Github } from 'lucide-react'

const features = [
  {
    icon: Rocket,
    iconColor: 'text-indigo-500',
    bgLight: 'bg-indigo-500/10',
    borderLight: 'border-indigo-500/20',
    title: 'Deploy Instantly',
    desc: 'Push your Laravel project from GitHub and watch it go live in minutes. No manual configuration needed.',
  },
  {
    icon: Database,
    iconColor: 'text-fuchsia-500',
    bgLight: 'bg-fuchsia-500/10',
    borderLight: 'border-fuchsia-500/20',
    title: 'Database Included',
    desc: 'Every project automatically provisions an isolated, secure MariaDB database out of the box.',
  },
  {
    icon: Globe,
    iconColor: 'text-blue-500',
    bgLight: 'bg-blue-500/10',
    borderLight: 'border-blue-500/20',
    title: 'Custom Subdomain',
    desc: 'Share your work instantly. Every project is assigned a unique subdomain the moment it deploys.',
  },
  {
    icon: Compass,
    iconColor: 'text-emerald-500',
    bgLight: 'bg-emerald-500/10',
    borderLight: 'border-emerald-500/20',
    title: 'PHP Version Control',
    desc: 'Seamlessly switch between PHP 8.0, 8.1, 8.2, 8.3, or 8.4 on a per-project basis with a single click.',
  },
  {
    icon: RefreshCw,
    iconColor: 'text-amber-500',
    bgLight: 'bg-amber-500/10',
    borderLight: 'border-amber-500/20',
    title: 'One-Click Redeploy',
    desc: 'Updated your code? Trigger a fresh deployment straight from the dashboard to keep your live app current.',
  },
  {
    icon: Terminal,
    iconColor: 'text-rose-500',
    bgLight: 'bg-rose-500/10',
    borderLight: 'border-rose-500/20',
    title: 'Artisan & Logs',
    desc: 'Run Artisan commands securely from your browser and monitor real-time container logs for easy debugging.',
  },
]

const steps = [
  { step: '01', title: 'Authenticate', desc: 'Securely log in using your student credentials provided by your instructor.' },
  { step: '02', title: 'Connect Repo', desc: 'Link your public GitHub repository containing your Laravel build.' },
  { step: '03', title: 'Launch', desc: 'Hit deploy and let the platform orchestrate your container, database, and routing.' },
]

export default function Landing() {
  const { token, user } = useAuthStore()
  const navigate = useNavigate()

  const isAdmin = user?.role === 'superadmin' || user?.role === 'admin'
  const dashboardPath = isAdmin ? '/admin/dashboard' : '/dashboard'

  return (
    <div className="min-h-screen bg-[#030305] text-white selection:bg-purple-500/30 overflow-hidden font-sans">
      {/* Dynamic Ambient Background Elements */}
      <div className="fixed inset-0 pointer-events-none z-0">
        <div className="absolute top-[-10%] left-[-10%] w-[50vw] h-[50vw] rounded-full bg-indigo-900/20 blur-[150px] mix-blend-screen opacity-50 animate-pulse-slow"></div>
        <div className="absolute bottom-[-10%] right-[-10%] w-[40vw] h-[40vw] rounded-full bg-fuchsia-900/20 blur-[150px] mix-blend-screen opacity-50 animate-pulse-slow font-delay-200"></div>
        {/* Subtle grid pattern overlay */}
        <div className="absolute inset-0 bg-[url('data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI0MCIgaGVpZ2h0PSI0MCI+PGRlZnM+PHBhdHRlcm4gaWQ9ImdyaWQiIHdpZHRoPSI0MCIgaGVpZ2h0PSI0MCIgcGF0dGVyblVuaXRzPSJ1c2VyU3BhY2VPblVzZSI+PHBhdHRoIGQ9Ik0gNDAgMCBMIDAgMCAwIDQwIiBmaWxsPSJub25lIiBzdHJva2U9InJnYmEoMjU1LDI1NSwyNTUsMC4wMykiIHN0cm9rZS13aWR0aD0iMSIvPjwvcGF0dGVybj48L2RlZnM+PHJlY3Qgd2lkdGg9Ijk5OTkiIGhlaWdodD0iOTk5OSIgZmlsbD0idXJsKCNncmlkKSIvPjwvc3ZnPg==')] [mask-image:linear-gradient(to_bottom,white_0%,transparent_100%)]"></div>
      </div>

      {/* Navbar View */}
      <header className="relative z-40 w-full backdrop-blur-md border-b border-white/5 bg-[#030305]/50 sticky top-0">
        <div className="max-w-7xl mx-auto px-6 h-20 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 bg-gradient-to-br from-indigo-500 to-purple-600 rounded-xl flex items-center justify-center shadow-lg shadow-indigo-500/20">
              <span className="text-xl font-bold text-white tracking-tighter">LP</span>
            </div>
            <span className="text-xl font-bold tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-white to-white/70">
              Laravel PaaS
            </span>
          </div>
          <div className="flex items-center gap-4">
            <a href="#features" className="hidden md:block text-sm font-medium text-slate-400 hover:text-white transition-colors">Features</a>
            <a href="#how-it-works" className="hidden md:block text-sm font-medium text-slate-400 hover:text-white transition-colors">How it works</a>
            
            <Link
              to={token ? dashboardPath : "/login"}
              className="ml-4 group relative inline-flex items-center justify-center px-6 py-2.5 text-sm font-semibold text-white transition-all duration-200 bg-white/5 border border-white/10 rounded-full overflow-hidden hover:bg-white/10 hover:border-white/20"
            >
              <span className="relative z-10 flex items-center gap-2">
                {token ? 'Go to Dashboard' : 'Sign In'}
                <ChevronRight className="w-4 h-4 group-hover:translate-x-1 transition-transform" />
              </span>
            </Link>
          </div>
        </div>
      </header>

      {/* Hero Section */}
      <section className="relative z-10 pt-32 pb-20 px-6 max-w-7xl mx-auto flex flex-col items-center text-center">
        <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-white/5 border border-white/10 backdrop-blur-sm mb-8 animate-pop-in">
          <span className="flex h-2 w-2 relative">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
            <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
          </span>
          <span className="text-sm font-medium text-slate-300">v2.0 Student Hosting Platform is live</span>
        </div>
        
        <h1 className="text-6xl md:text-8xl font-black tracking-tighter mb-8 leading-[1.1] max-w-5xl">
          Deploy Laravel <br className="hidden md:block" />
          <span className="text-transparent bg-clip-text bg-gradient-to-r from-indigo-400 via-purple-400 to-fuchsia-400">
            Without the Headache.
          </span>
        </h1>
        
        <p className="text-lg md:text-xl text-slate-400 max-w-2xl leading-relaxed mb-12">
          The premium self-hosted PaaS tailored for students. Connect your GitHub repository, select your PHP runtime, and get a production-ready URL instantly.
        </p>

        <div className="flex flex-col sm:flex-row items-center gap-5 w-full sm:w-auto">
          <Link
            to={token ? dashboardPath : "/login"}
            className="group relative flex items-center justify-center gap-3 w-full sm:w-auto px-8 py-4 text-white font-bold text-lg rounded-2xl bg-gradient-to-r from-indigo-600 to-purple-600 hover:from-indigo-500 hover:to-purple-500 shadow-xl shadow-indigo-500/25 transition-all outline-none focus:ring-2 focus:ring-offset-2 focus:ring-offset-[#030305] focus:ring-indigo-500 transform hover:-translate-y-1"
          >
            {token ? 'Open Dashboard' : 'Start Deploying'}
            <ArrowRight className="w-5 h-5 group-hover:translate-x-1 transition-transform" />
          </Link>
          
          <a
            href="https://github.com"
            target="_blank"
            rel="noreferrer"
            className="flex items-center justify-center gap-3 w-full sm:w-auto px-8 py-4 text-white font-semibold text-lg rounded-2xl bg-[#111114] border border-white/10 hover:bg-[#1a1a1f] hover:border-white/20 transition-all outline-none focus:ring-2 focus:ring-offset-2 focus:ring-offset-[#030305] focus:ring-slate-500"
          >
            <Github className="w-5 h-5" />
            View Documentation
          </a>
        </div>
        
        {/* Mockup Preview */}
        <div className="w-full max-w-5xl mt-24 relative perspective-[2000px]">
          <div className="absolute inset-0 bg-gradient-to-t from-[#030305] via-[#030305]/80 to-transparent z-10 bottom-0 top-1/2"></div>
          <div className="relative rounded-t-3xl border border-white/10 bg-[#0f0f12] overflow-hidden shadow-2xl shadow-indigo-500/10 transform rotate-x-12 translate-y-10 scale-[0.98]">
            {/* Fake macOS Window Header */}
            <div className="h-12 border-b border-white/5 bg-[#111114] flex items-center px-4 gap-2">
              <div className="flex gap-1.5">
                <div className="w-3 h-3 rounded-full bg-rose-500/80"></div>
                <div className="w-3 h-3 rounded-full bg-amber-500/80"></div>
                <div className="w-3 h-3 rounded-full bg-emerald-500/80"></div>
              </div>
              <div className="mx-auto flex items-center gap-2 px-3 py-1 rounded-md bg-white/5 text-xs text-slate-400 font-mono">
                <Globe className="w-3 h-3" />
                your-project.student-paas.local
              </div>
            </div>
            {/* Fake Content area */}
            <div className="p-8 grid grid-cols-3 gap-6 opacity-80 h-[400px]">
               <div className="col-span-2 space-y-4">
                  <div className="h-24 rounded-2xl bg-white/5 border border-white/5"></div>
                  <div className="grid grid-cols-2 gap-4">
                     <div className="h-32 rounded-2xl bg-white/5 border border-white/5"></div>
                     <div className="h-32 rounded-2xl bg-white/5 border border-white/5"></div>
                  </div>
               </div>
               <div className="col-span-1 space-y-4">
                  <div className="h-40 rounded-2xl bg-indigo-500/10 border border-indigo-500/20"></div>
                  <div className="h-16 rounded-2xl bg-white/5 border border-white/5"></div>
               </div>
            </div>
          </div>
        </div>
      </section>

      {/* Features Grid */}
      <section id="features" className="relative z-10 py-32 px-6 bg-[#0a0a0c]">
        <div className="max-w-7xl mx-auto">
          <div className="text-center mb-20">
            <h2 className="text-sm font-bold tracking-widest text-indigo-400 uppercase mb-3">Platform Features</h2>
            <h3 className="text-4xl md:text-5xl font-bold text-white mb-6">Everything you need to ship.</h3>
            <p className="text-lg text-slate-400 max-w-2xl mx-auto">
              We took the complexity out of server management so you can focus entirely on writing great Laravel code.
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {features.map((item, idx) => {
              const Icon = item.icon
              return (
                <div key={idx} className="group p-8 rounded-3xl bg-[#111114] border border-white/[0.03] hover:border-white/[0.08] transition-all duration-300 hover:-translate-y-1 hover:shadow-2xl hover:shadow-black/50">
                  <div className={`w-14 h-14 rounded-2xl ${item.bgLight} ${item.borderLight} border flex items-center justify-center mb-6`}>
                    <Icon className={`w-7 h-7 ${item.iconColor}`} />
                  </div>
                  <h4 className="text-xl font-bold text-white mb-3">{item.title}</h4>
                  <p className="text-slate-400 text-sm leading-relaxed">{item.desc}</p>
                </div>
              )
            })}
          </div>
        </div>
      </section>

      {/* How it Works Workflow */}
      <section id="how-it-works" className="relative z-10 py-32 px-6 overflow-hidden">
        <div className="max-w-7xl mx-auto">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-16 items-center">
            
            <div className="order-2 lg:order-1 relative">
               <div className="absolute inset-0 bg-gradient-to-tr from-indigo-500/10 to-purple-500/10 rounded-3xl blur-3xl"></div>
               <div className="relative aspect-square border border-white/10 rounded-3xl bg-[#0a0a0c]/80 backdrop-blur top-0 p-8 flex flex-col justify-between">
                  {/* Fake Code representation block */}
                  <div className="space-y-4">
                     <div className="flex items-center gap-3">
                        <Terminal className="w-5 h-5 text-slate-400" />
                        <span className="font-mono text-sm text-slate-300">git clone ...</span>
                     </div>
                     <div className="p-4 rounded-xl bg-black/50 border border-white/5 font-mono text-xs text-emerald-400 leading-loose">
                        &gt; starting build process...<br/>
                        &gt; configuring php 8.2...<br/>
                        &gt; installing composer dependencies...<br/>
                        &gt; provisioning mariadb...<br/>
                        <span className="text-indigo-400">&gt; running migrations...</span><br/>
                        &gt; configuring nginx routing...<br/>
                        <span className="text-white mt-2 block font-bold">✓ Project Live: https://app.paas.local</span>
                     </div>
                  </div>
                  
                  {/* Deployment status card */}
                  <div className="p-5 rounded-2xl bg-indigo-500/10 border border-indigo-500/20 flex items-center gap-4">
                     <div className="w-12 h-12 rounded-full bg-indigo-500/20 flex items-center justify-center">
                        <CheckCircle2 className="w-6 h-6 text-indigo-400" />
                     </div>
                     <div>
                        <h5 className="text-white font-bold">Deployment Successful</h5>
                        <p className="text-slate-400 text-sm">Takes less than 30 seconds average</p>
                     </div>
                  </div>
               </div>
            </div>

            <div className="order-1 lg:order-2">
              <h2 className="text-sm font-bold tracking-widest text-fuchsia-400 uppercase mb-3">Workflow</h2>
              <h3 className="text-4xl md:text-5xl font-bold text-white mb-8">From repository to production instantly.</h3>
              
              <div className="space-y-8">
                {steps.map((step, idx) => (
                  <div key={idx} className="flex gap-6 group">
                    <div className="flex flex-col items-center">
                      <div className="w-12 h-12 rounded-2xl bg-[#111114] border border-white/10 flex items-center justify-center font-mono font-bold text-slate-300 group-hover:bg-white border group-hover:border-white group-hover:text-black transition-all">
                        {step.step}
                      </div>
                      {idx !== steps.length - 1 && (
                        <div className="w-px h-16 bg-gradient-to-b from-white/10 to-transparent mt-2"></div>
                      )}
                    </div>
                    <div className="pt-2">
                       <h4 className="text-xl font-bold text-white mb-2">{step.title}</h4>
                       <p className="text-slate-400 text-base">{step.desc}</p>
                    </div>
                  </div>
                ))}
              </div>
            </div>

          </div>
        </div>
      </section>

      {/* CTA Box */}
      <section className="relative z-10 py-24 px-6">
        <div className="max-w-5xl mx-auto">
          <div className="relative rounded-[2.5rem] overflow-hidden bg-gradient-to-r from-indigo-900/50 to-purple-900/50 border border-white/10 p-12 md:p-20 text-center">
             <div className="absolute inset-0 bg-[url('data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI0MCIgaGVpZ2h0PSI0MCI+PGRlZnM+PHBhdHRlcm4gaWQ9ImdyaWQiIHdpZHRoPSI0MCIgaGVpZ2h0PSI0MCIgcGF0dGVyblVuaXRzPSJ1c2VyU3BhY2VPblVzZSI+PHBhdHRoIGQ9Ik0gNDAgMCBMIDAgMCAwIDQwIiBmaWxsPSJub25lIiBzdHJva2U9InJnYmEoMjU1LDI1NSwyNTUsMC4wMSkiIHN0cm9rZS13aWR0aD0iMSIvPjwvcGF0dGVybj48L2RlZnM+PHJlY3Qgd2lkdGg9Ijk5OTkiIGhlaWdodD0iOTk5OSIgZmlsbD0idXJsKCNncmlkKSIvPjwvc3ZnPg==')]"></div>
             
             <div className="relative z-10">
               <h2 className="text-4xl md:text-5xl font-bold text-white mb-6">Ready to ship?</h2>
               <p className="text-xl text-slate-300 max-w-2xl mx-auto mb-10">
                 Stop wrestling with Nginx configurations and Dockerfiles. Let the platform do the heavy lifting.
               </p>
               
               <Link
                 to={token ? dashboardPath : "/login"}
                 className="inline-flex items-center gap-2 px-8 py-4 bg-white text-black font-bold text-lg rounded-2xl hover:bg-slate-200 hover:scale-105 transition-all shadow-xl shadow-white/10"
               >
                 {token ? "Return to Dashboard" : "Sign in to deploy"}
                 <ArrowRight className="w-5 h-5" />
               </Link>
             </div>
          </div>
        </div>
      </section>

      {/* Modern Footer */}
      <footer className="relative z-10 border-t border-white/5 bg-[#030305] pt-16 pb-8 px-6">
         <div className="max-w-7xl mx-auto flex flex-col md:flex-row items-center justify-between gap-6">
            <div className="flex items-center gap-3">
               <div className="w-8 h-8 bg-white/5 rounded-lg flex items-center justify-center">
                 <span className="text-sm font-bold text-white">LP</span>
               </div>
               <span className="text-slate-400 font-medium tracking-tight">Laravel PaaS</span>
            </div>
            
            <p className="text-slate-500 text-sm">
               &copy; {new Date().getFullYear()} Advanced Student Hosting. All rights reserved.
            </p>
         </div>
      </footer>
    </div>
  )
}
