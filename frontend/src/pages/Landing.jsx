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
    icon: (
      <svg className="w-8 h-8 text-primary-400" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" d="M15.59 14.37a6 6 0 01-5.84 7.38v-4.8m5.84-2.58a14.98 14.98 0 006.16-12.12A14.98 14.98 0 009.631 8.41m5.96 5.96a14.926 14.926 0 01-5.841 2.58m-.119-8.54a6 6 0 00-7.381 5.84h4.8m2.581-5.84a14.927 14.927 0 00-2.58 5.84m2.699-2.7c-.91.91-1.076 2.05-.365 2.71c.71.71 1.85.545 2.76-.365c.911-.911 1.077-2.051.366-2.711c-.71-.71-1.85-.545-2.761.366z" />
      </svg>
    ),
    title: 'Deploy Instantly',
    desc: 'Push your Laravel project from GitHub and it will be live in minutes, automatically.',
  },
  {
    icon: (
      <svg className="w-8 h-8 text-purple-400" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" d="M20.25 6.375c0 2.278-3.694 4.125-8.25 4.125S3.75 8.653 3.75 6.375m16.5 0c0-2.278-3.694-4.125-8.25-4.125S3.75 4.097 3.75 6.375m16.5 0v11.25c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125V6.375m16.5 0v3.75m-16.5-3.75v3.75m16.5 0v3.75M3.75 10.125v3.75m16.5 0v3.75M3.75 13.875v3.75" />
      </svg>
    ),
    title: 'Database Included',
    desc: 'Each project gets its own isolated MariaDB database. No manual setup needed.',
  },
  {
    icon: (
      <svg className="w-8 h-8 text-blue-400" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 21a9.041 9.041 0 01-2.427-.328 12.035 12.035 0 01-5.264-3.551 12.035 12.035 0 01-3.551-5.264A9.041 9.041 0 011 9.573a9.041 9.041 0 01.328-2.427 12.035 12.035 0 013.551-5.264 12.035 12.035 0 015.264-3.551A9.041 9.041 0 0112 1a9.041 9.041 0 012.427.328 12.035 12.035 0 015.264 3.551 12.035 12.035 0 013.551 5.264 9.041 9.041 0 01.328 2.427 9.041 9.041 0 01-.328 2.427 12.035 12.035 0 01-3.551 5.264 12.035 12.035 0 01-5.264 3.551A9.041 9.041 0 0112 21z" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 1a9 9 0 00-9 9m9-9a9 9 0 019 9m-9-9v18m0-18C8.5 1 5.5 4 5.5 10s3 9 6.5 9m0-18c3.5 0 6.5 3 6.5 9s-3 9-6.5 9M1.2 12.5h21.6M2.5 7h19M2.5 17h19" />
      </svg>
    ),
    title: 'Custom Subdomain',
    desc: 'Every project is accessible via a unique subdomain right after deployment.',
  },
  {
    icon: (
      <svg className="w-8 h-8 text-emerald-400" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" d="M21 7.5l-9-5.25L3 7.5m18 0l-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-10.5v10.5" />
      </svg>
    ),
    title: 'PHP Version Control',
    desc: 'Choose PHP 8.0 – 8.4 per project. We handle the container configuration for you.',
  },
  {
    icon: (
      <svg className="w-8 h-8 text-amber-400" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" />
      </svg>
    ),
    title: 'One-Click Redeploy',
    desc: 'Update your app by triggering a redeploy anytime from the dashboard.',
  },
  {
    icon: (
      <svg className="w-8 h-8 text-rose-400" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" d="M11.42 15.17L17.25 21A2.652 2.652 0 0021 17.25l-5.83-5.83m-5.83 5.83a2.652 2.652 0 11-3.75-3.75l5.83-5.83m5.83 5.83V9a3 3 0 00-3-3H9m-6 3l3.181 3.182a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" />
      </svg>
    ),
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
          <div className="w-9 h-9 bg-primary-600 rounded-xl flex items-center justify-center shadow-lg shadow-primary-600/20 text-white">
            <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={2.5} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M15.59 14.37a6 6 0 01-5.84 7.38v-4.8m5.84-2.58a14.98 14.98 0 006.16-12.12A14.98 14.98 0 009.631 8.41m5.96 5.96a14.926 14.926 0 01-5.841 2.58m-.119-8.54a6 6 0 00-7.381 5.84h4.8m2.581-5.84a14.927 14.927 0 00-2.58 5.84m2.699-2.7c-.91.91-1.076 2.05-.365 2.71c.71.71 1.85.545 2.76-.365" />
            </svg>
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
