// ===========================================
// Login Page (Modern Minimalist Refactor)
// ===========================================

import { useState, useEffect } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import useAuthStore from '../stores/authStore'
import { ArrowRight, Loader2, ArrowLeft, Terminal } from 'lucide-react'

function Login() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [validationErrors, setValidationErrors] = useState({})
  
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
    
    // Minimalistic Modern Validation
    const errors = {}
    if (!email.trim()) errors.email = 'Email address is required'
    if (!password) errors.password = 'Password is required'
    
    if (Object.keys(errors).length > 0) {
      setValidationErrors(errors)
      return
    }

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
    <div className="min-h-screen flex items-center justify-center p-6 bg-[#09090b] text-[#f8fafc] font-sans antialiased selection:bg-indigo-500/30">
      {/* Background Layer */}
      <div className="fixed inset-0 pointer-events-none opacity-[0.03] bg-[url('https://grainy-gradients.vercel.app/noise.svg')]"></div>

      <div className="relative z-10 w-full max-w-[400px]">
        <Link to="/" className="inline-flex items-center gap-2 text-xs font-bold uppercase tracking-widest text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white transition-colors mb-12 group">
          <ArrowLeft className="w-3 h-3 group-hover:-translate-x-1 transition-transform" />
          Back to Home
        </Link>
        
        {/* Identity Head */}
        <div className="mb-12">
          <div className="mb-8 items-start justify-center flex flex-col">
             <h1 className="text-2xl font-bold tracking-tight text-slate-900 dark:text-white leading-none">Sign In</h1>
             <p className="text-xs text-slate-600 dark:text-slate-400 font-medium mt-2">Enter your credentials to access your account.</p>
          </div>
        </div>
        
        {/* Authentication Interface */}
        <form onSubmit={handleSubmit} noValidate className="space-y-8 bg-[#0d0d12] border border-slate-200 dark:border-white/5 p-10 rounded-[2rem] shadow-2xl relative overflow-hidden group/form">
          <div className="absolute top-0 right-0 p-4 opacity-10">
             <Terminal size={120} className="text-indigo-500 -mr-16 -mt-16 rotate-12" />
          </div>

          <div className="space-y-6 relative z-10">
            <div className="space-y-2">
              <label htmlFor="email" className="block text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-[0.2em] ml-1">
                Email Address
              </label>
              <input
                id="email"
                type="email"
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value)
                  if(validationErrors.email) setValidationErrors(prev => ({...prev, email: null}))
                }}
                className={`w-full px-5 py-4 bg-black/40 border border-slate-300 dark:border-white/10 text-slate-900 dark:text-white rounded-xl focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all duration-300 placeholder-slate-700 outline-none text-sm font-medium ${validationErrors.email ? '!border-rose-500/50 focus:!ring-rose-500/20 focus:!border-rose-500' : ''}`}
                placeholder="name@example.com"
                autoFocus
              />
              {validationErrors.email && (
                 <p className="text-[10px] text-rose-400 font-bold uppercase tracking-widest pl-1 mt-1 animate-pop-in">{validationErrors.email}</p>
              )}
            </div>
            
            <div className="space-y-2">
              <label htmlFor="password" className="block text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-[0.2em] ml-1">
                Password
              </label>
              <input
                id="password"
                type="password"
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value)
                  if(validationErrors.password) setValidationErrors(prev => ({...prev, password: null}))
                }}
                className={`w-full px-5 py-4 bg-black/40 border border-slate-300 dark:border-white/10 text-slate-900 dark:text-white rounded-xl focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all duration-300 placeholder-slate-700 outline-none text-sm font-medium ${validationErrors.password ? '!border-rose-500/50 focus:!ring-rose-500/20 focus:!border-rose-500' : ''}`}
                placeholder="••••••••"
              />
              {validationErrors.password && (
                 <p className="text-[10px] text-rose-400 font-bold uppercase tracking-widest pl-1 mt-1 animate-pop-in">{validationErrors.password}</p>
              )}
            </div>
            
            <div className="pt-4">
               <button
                 type="submit"
                 disabled={isLoading}
                 className="w-full relative px-8 py-5 bg-white text-black font-black text-xs uppercase tracking-[0.3em] rounded-xl hover:bg-slate-200 transition-all disabled:opacity-50 disabled:cursor-not-allowed group/btn active:scale-95 shadow-xl shadow-white/5"
               >
                 <div className="flex items-center justify-center gap-3">
                   {isLoading ? (
                     <div className="flex items-center gap-2">
                       <div className="w-1.5 h-1.5 bg-black rounded-full animate-bounce [animation-delay:-0.3s]"></div>
                       <div className="w-1.5 h-1.5 bg-black rounded-full animate-bounce [animation-delay:-0.15s]"></div>
                       <div className="w-1.5 h-1.5 bg-black rounded-full animate-bounce"></div>
                       <span>Logging In</span>
                     </div>
                   ) : (
                     <>
                       Sign In
                       <ArrowRight className="w-4 h-4 group-hover/btn:translate-x-1 transition-transform" />
                     </>
                   )}
                 </div>
               </button>
            </div>
          </div>
        </form>
        
        {/* Provisions Footer */}
        <div className="mt-12 flex items-center justify-between px-2">
           <p className="text-[10px] font-black text-slate-600 uppercase tracking-[0.2em]">Platform Version 2.8</p>
           <Link to="/" className="text-[10px] font-black text-indigo-400 hover:text-indigo-300 transition-colors uppercase tracking-[0.2em]">Support</Link>
        </div>
      </div>
    </div>
  )
}

export default Login