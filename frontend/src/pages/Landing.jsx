// ===========================================
// Landing Page
// ===========================================
// Public overview page with login CTA
// ===========================================

import { useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import useAuthStore from '../stores/authStore'

const features = [
  {
    icon: '🚀',
    title: 'Deploy Instantly',
    desc: 'Push your Laravel project from GitHub and it will be live in minutes, automatically.',
  },
  {
    icon: '🗄️',
    title: 'Database Included',
    desc: 'Each project gets its own isolated MariaDB database. No manual setup needed.',
  },
  {
    icon: '🌐',
    title: 'Custom Subdomain',
    desc: 'Every project is accessible via a unique subdomain right after deployment.',
  },
  {
    icon: '📦',
    title: 'PHP Version Control',
    desc: 'Choose PHP 8.0 – 8.4 per project. We handle the container configuration for you.',
  },
  {
    icon: '🔄',
    title: 'One-Click Redeploy',
    desc: 'Update your app by triggering a redeploy anytime from the dashboard.',
  },
  {
    icon: '🛠️',
    title: 'Artisan & Logs',
    desc: 'Run Artisan commands and view real-time container logs directly from the panel.',
  },
]

const steps = [
  { number: '01', title: 'Login', desc: 'Log in with your student account provided by the admin.' },
  { number: '02', title: 'Connect GitHub', desc: 'Paste your public GitHub repository URL and choose a branch.' },
  { number: '03', title: 'Deploy', desc: 'Hit Deploy — your app is cloned, built, and served on a subdomain.' },
]

export default function Landing() {
  const { token, user } = useAuthStore()
  const navigate = useNavigate()

  // If already logged in, redirect to dashboard
  useEffect(() => {
    if (token) {
      const isAdmin = user?.role === 'superadmin' || user?.role === 'admin'
      navigate(isAdmin ? '/admin/dashboard' : '/dashboard', { replace: true })
    }
  }, [token, user, navigate])

  return (
    <div className="min-h-screen bg-gradient-to-br from-slate-900 via-slate-800 to-slate-900 text-white">
      {/* Background blobs */}
      <div className="fixed inset-0 overflow-hidden pointer-events-none">
        <div className="absolute -top-1/3 -left-1/4 w-2/3 h-2/3 bg-primary-600/10 rounded-full blur-3xl" />
        <div className="absolute -bottom-1/3 -right-1/4 w-2/3 h-2/3 bg-purple-600/10 rounded-full blur-3xl" />
      </div>

      {/* Navbar */}
      <header className="relative z-10 flex items-center justify-between px-6 py-5 max-w-6xl mx-auto">
        <div className="flex items-center gap-3">
          <div className="w-9 h-9 bg-primary-600 rounded-xl flex items-center justify-center shadow-lg shadow-primary-600/20">
            <span className="text-lg">🚀</span>
          </div>
          <span className="text-xl font-bold tracking-tight">Laravel PaaS</span>
        </div>
        <Link
          to="/login"
          className="btn btn-primary px-5 py-2 text-sm font-semibold rounded-xl"
        >
          Sign In
        </Link>
      </header>

      {/* Hero */}
      <section className="relative z-10 text-center px-6 pt-20 pb-24 max-w-4xl mx-auto">
        <span className="inline-flex items-center gap-2 text-xs font-semibold text-primary-400 bg-primary-600/10 border border-primary-600/20 rounded-full px-4 py-1.5 mb-6">
          <span className="w-2 h-2 rounded-full bg-primary-400 animate-pulse" />
          Student Hosting Platform
        </span>
        <h1 className="text-5xl sm:text-6xl font-extrabold leading-tight tracking-tight mb-6">
          Deploy your Laravel app
          <br />
          <span className="text-transparent bg-clip-text bg-gradient-to-r from-primary-400 to-purple-400">
            in minutes, not hours.
          </span>
        </h1>
        <p className="text-slate-400 text-lg sm:text-xl leading-relaxed max-w-2xl mx-auto mb-10">
          A self-hosted PaaS built for students. Connect your GitHub repo,
          pick a PHP version, and get a live URL — everything else is handled for you.
        </p>
        <div className="flex flex-col sm:flex-row items-center justify-center gap-4">
          <Link
            to="/login"
            className="btn btn-primary px-8 py-3.5 text-base font-semibold rounded-xl w-full sm:w-auto"
          >
            Get Started →
          </Link>
          <a
            href="#how-it-works"
            className="text-slate-400 hover:text-white transition-colors text-sm font-medium"
          >
            How it works ↓
          </a>
        </div>
      </section>

      {/* Features */}
      <section className="relative z-10 px-6 pb-24 max-w-6xl mx-auto">
        <h2 className="text-center text-2xl font-bold text-slate-200 mb-12">
          Everything you need to ship
        </h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-5">
          {features.map((f) => (
            <div
              key={f.title}
              className="bg-slate-800/50 border border-slate-700/50 rounded-2xl p-6 hover:border-primary-600/40 hover:bg-slate-800/80 transition-all"
            >
              <div className="text-3xl mb-4">{f.icon}</div>
              <h3 className="text-white font-semibold text-base mb-2">{f.title}</h3>
              <p className="text-slate-400 text-sm leading-relaxed">{f.desc}</p>
            </div>
          ))}
        </div>
      </section>

      {/* How it works */}
      <section id="how-it-works" className="relative z-10 px-6 pb-28 max-w-4xl mx-auto">
        <h2 className="text-center text-2xl font-bold text-slate-200 mb-12">How it works</h2>
        <div className="relative flex flex-col gap-8">
          {/* Vertical line */}
          <div className="absolute left-[2.1rem] top-4 bottom-4 w-px bg-slate-700 hidden sm:block" />
          {steps.map((s) => (
            <div key={s.number} className="flex items-start gap-6">
              <div className="shrink-0 w-[4.2rem] h-[4.2rem] rounded-2xl bg-primary-600/20 border border-primary-600/30 flex flex-col items-center justify-center z-10">
                <span className="text-xs text-primary-400 font-bold">{s.number}</span>
              </div>
              <div className="pt-1">
                <h3 className="text-white font-semibold text-base mb-1">{s.title}</h3>
                <p className="text-slate-400 text-sm leading-relaxed">{s.desc}</p>
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* CTA Banner */}
      <section className="relative z-10 px-6 pb-24 max-w-4xl mx-auto">
        <div className="bg-gradient-to-r from-primary-600/20 to-purple-600/20 border border-primary-600/20 rounded-3xl px-8 py-12 text-center">
          <h2 className="text-3xl font-bold mb-4">Ready to deploy?</h2>
          <p className="text-slate-400 mb-8 max-w-md mx-auto">
            Sign in with your student account and launch your first Laravel project today.
          </p>
          <Link
            to="/login"
            className="btn btn-primary px-8 py-3.5 text-base font-semibold rounded-xl inline-block"
          >
            Sign In to Get Started
          </Link>
        </div>
      </section>

      {/* Footer */}
      <footer className="relative z-10 border-t border-slate-800 px-6 py-8 max-w-6xl mx-auto text-center text-slate-600 text-sm">
        © {new Date().getFullYear()} Laravel PaaS — Student Hosting Platform
      </footer>
    </div>
  )
}
