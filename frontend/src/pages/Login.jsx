// ===========================================
// Login Page
// ===========================================

import { useState, useEffect } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import useAuthStore from '../stores/authStore'
import { ArrowRight, Loader2, ArrowLeft } from 'lucide-react'

function Login() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  
  const login = useAuthStore((state) => state.login)
  const token = useAuthStore((state) => state.token)
  const user = useAuthStore((state) => state.user)
  const navigate = useNavigate()

  useEffect(() => {
    if (token && user) {
      const isAdmin = user.role === 'superadmin' || user.role === 'admin'
      navigate(isAdmin ? '/admin/dashboard' : '/dashboard', { replace: true })
    }
  }, [token, user, navigate])
  
  const handleSubmit = async (e) => {
    e.preventDefault()
    setIsLoading(true)
    
    try {
      const user = await login(email, password)
      toast.success(`Welcome back, ${user.name}!`)
      
      // Redirect based on role
      if (user.role === 'superadmin' || user.role === 'admin') {
        navigate('/admin/dashboard')
      } else {
        navigate('/dashboard')
      }
    } catch (error) {
      toast.error(error.response?.data?.error || 'Login failed')
    } finally {
      setIsLoading(false)
    }
  }
  
  return (
    <div className="min-h-screen flex items-center justify-center p-4 bg-[#030305] text-white overflow-hidden relative font-sans">
      {/* Dynamic Ambient Background Elements */}
      <div className="absolute inset-0 pointer-events-none z-0">
        <div className="absolute top-[-20%] left-[-10%] w-[60vw] h-[60vw] rounded-full bg-indigo-900/10 blur-[150px] mix-blend-screen opacity-50 animate-pulse-slow"></div>
        <div className="absolute bottom-[-10%] right-[-10%] w-[50vw] h-[50vw] rounded-full bg-fuchsia-900/10 blur-[150px] mix-blend-screen opacity-50 animate-pulse-slow font-delay-200"></div>
        <div className="absolute inset-0 bg-[url('data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI0MCIgaGVpZ2h0PSI0MCI+PGRlZnM+PHBhdHRlcm4gaWQ9ImdyaWQiIHdpZHRoPSI0MCIgaGVpZ2h0PSI0MCIgcGF0dGVyblVuaXRzPSJ1c2VyU3BhY2VPblVzZSI+PHBhdHRoIGQ9Ik0gNDAgMCBMIDAgMCAwIDQwIiBmaWxsPSJub25lIiBzdHJva2U9InJnYmEoMjU1LDI1NSwyNTUsMC4wMykiIHN0cm9rZS13aWR0aD0iMSIvPjwvcGF0dGVybj48L2RlZnM+PHJlY3Qgd2lkdGg9Ijk5OTkiIGhlaWdodD0iOTk5OSIgZmlsbD0idXJsKCNncmlkKSIvPjwvc3ZnPg==')] [mask-image:linear-gradient(to_bottom,white_0%,transparent_100%)]"></div>
      </div>
      
      <div className="relative z-10 w-full max-w-[420px] animate-pop-in">
        <Link to="/" className="inline-flex items-center gap-2 text-sm text-slate-400 hover:text-white transition-colors mb-10 group">
          <ArrowLeft className="w-4 h-4 group-hover:-translate-x-1 transition-transform" />
          Back to home
        </Link>
        
        {/* Logo View */}
        <div className="text-center mb-10">
          <div className="inline-flex items-center justify-center w-16 h-16 bg-gradient-to-br from-indigo-500 to-purple-600 rounded-2xl mb-5 shadow-lg shadow-indigo-500/20 shadow-[0_0_40px_rgba(99,102,241,0.2)]">
             <span className="text-2xl font-black text-white tracking-tighter">LP</span>
          </div>
          <h1 className="text-3xl font-bold bg-clip-text text-transparent bg-gradient-to-b from-white to-white/70">Welcome back</h1>
          <p className="text-slate-400 mt-2 text-sm">Sign in to your student workspace</p>
        </div>
        
        {/* Login Form */}
        <form onSubmit={handleSubmit} className="backdrop-blur-xl bg-[#0a0a0c]/80 border border-white/10 rounded-3xl p-8 shadow-2xl">
          <div className="space-y-6">
            <div>
              <label htmlFor="email" className="block text-xs font-bold text-slate-400 uppercase tracking-widest mb-2 ml-1">
                Email Address
              </label>
              <input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="w-full px-5 py-3.5 bg-white/5 border border-white/10 text-white rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition-all duration-200 placeholder-slate-600 outline-none"
                placeholder="student@university.edu"
                required
                autoFocus
              />
            </div>
            
            <div>
              <label htmlFor="password" className="block text-xs font-bold text-slate-400 uppercase tracking-widest mb-2 ml-1">
                Password
              </label>
              <input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full px-5 py-3.5 bg-white/5 border border-white/10 text-white rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition-all duration-200 placeholder-slate-600 outline-none"
                placeholder="••••••••"
                required
              />
            </div>
            
            <div className="pt-2">
               <button
                 type="submit"
                 disabled={isLoading}
                 className="w-full relative group overflow-hidden rounded-xl bg-white text-black font-bold text-base transition-all disabled:opacity-50 disabled:cursor-not-allowed h-14"
               >
                 <span className="absolute inset-0 w-full h-full bg-gradient-to-r from-indigo-100 to-purple-100 opacity-0 group-hover:opacity-100 transition-opacity"></span>
                 <span className="relative flex items-center justify-center gap-2">
                   {isLoading ? (
                     <>
                       <Loader2 className="w-5 h-5 animate-spin" />
                       Authenticating...
                     </>
                   ) : (
                     <>
                       Sign In
                       <ArrowRight className="w-5 h-5 group-hover:translate-x-1 transition-transform" />
                     </>
                   )}
                 </span>
               </button>
            </div>
          </div>
        </form>
        
        {/* Footer */}
        <p className="text-center mt-8 text-slate-500 text-sm bg-white/5 border border-white/5 py-3 rounded-xl backdrop-blur-sm">
          No account? <span className="text-slate-300">Contact your instructor to get provisioned.</span>
        </p>
      </div>
      
      <style>{`
        @keyframes pop-in {
          0% { opacity: 0; transform: scale(0.95) translateY(10px); }
          100% { opacity: 1; transform: scale(1) translateY(0); }
        }
        .animate-pop-in {
          animation: pop-in 0.6s cubic-bezier(0.16, 1, 0.3, 1) both;
        }
      `}</style>
    </div>
  )
}

export default Login

