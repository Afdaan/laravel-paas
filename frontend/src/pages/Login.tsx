import React, { useState, useEffect } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { toast } from 'sonner'
import { systemAPI } from '../services/api'
import useAuthStore from '../stores/authStore'
import useTranslation from '../lib/useTranslation'
import { ArrowRight, ArrowLeft, Terminal, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Sun, Moon } from 'lucide-react'
import { useTheme } from '@/components/ThemeProvider'

function Login() {
  const { t } = useTranslation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [validationErrors, setValidationErrors] = useState<Record<string, string | null>>({})
  
  const login = useAuthStore((state) => state.login)
  const token = useAuthStore((state) => state.token)
  const user = useAuthStore((state) => state.user)
  const { theme, setTheme } = useTheme()
  const navigate = useNavigate()

  useEffect(() => {
    // Check if system is initialized
    const checkInit = async () => {
      try {
        const { data } = await systemAPI.getInitStatus()
        if (!data.is_initialized) {
          navigate('/setup', { replace: true })
        }
      } catch (e) {}
    }
    checkInit()

    if (token && user) {
      const isAdmin = user.role === 'superadmin' || user.role === 'admin'
      navigate(isAdmin ? '/admin/dashboard' : '/dashboard', { replace: true })
    }
  }, [token, user, navigate])
  
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    
    const errors: Record<string, string | null> = {}
    if (!email.trim()) errors.email = t('login.emailRequired')
    if (!password) errors.password = t('login.passwordRequired')
    
    if (Object.keys(errors).length > 0) {
      setValidationErrors(errors)
      return
    }

    setIsLoading(true)
    
    try {
      const user = await login(email, password)
      toast.success(t('login.welcomeBack', { name: user.name }))
      
      if (user.role === 'superadmin' || user.role === 'admin') {
        navigate('/admin/dashboard')
      } else {
        navigate('/dashboard')
      }
    } catch (error: any) {
      toast.error(error.response?.data?.error || t('login.failed'))
    } finally {
      setIsLoading(false)
    }
  }
  
  const isDark = theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)

  return (
    <div className="min-h-screen flex items-center justify-center p-6 bg-background text-foreground font-sans relative text-sm">
      {/* Floating Theme Toggle */}
      <div className="absolute top-6 right-6">
         <Button
            variant="outline"
            size="icon"
            onClick={() => setTheme(isDark ? 'light' : 'dark')}
            className="rounded-full shadow-sm"
            title={t('common.theme')}
         >
            {isDark ? <Sun className="w-4 h-4 translate-y-0" /> : <Moon className="w-4 h-4 translate-y-0" />}
         </Button>
      </div>
      <div className="w-full max-w-md">
        <Button variant="ghost" className="mb-8" render={<Link to="/" className="text-muted-foreground group" />} nativeButton={false}>
          <ArrowLeft className="w-4 h-4 mr-2 group-hover:-translate-x-1 transition-transform" />
          {t('login.backToHome')}
        </Button>
        
        <Card>
          <CardHeader className="space-y-1">
             <div className="flex items-center justify-center mb-4">
                 <div className="w-12 h-12 bg-primary text-primary-foreground rounded-lg flex items-center justify-center flex-shrink-0">
                    <Terminal className="w-6 h-6" />
                 </div>
             </div>
             <CardTitle className="text-2xl text-center">{t('login.signIn')}</CardTitle>
             <CardDescription className="text-center">{t('login.desc')}</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="email" className={validationErrors.email ? "text-destructive" : ""}>{t('login.email')}</Label>
                <Input
                  id="email"
                  type="email"
                  value={email}
                  onChange={(e) => {
                    setEmail(e.target.value)
                    if(validationErrors.email) setValidationErrors(prev => ({...prev, email: null}))
                  }}
                  className={validationErrors.email ? "border-destructive focus-visible:ring-destructive" : ""}
                  placeholder="name@example.com"
                  autoFocus
                />
                {validationErrors.email && (
                   <p className="text-xs text-destructive font-medium">{validationErrors.email}</p>
                )}
              </div>
              
              <div className="space-y-2">
                <Label htmlFor="password" className={validationErrors.password ? "text-destructive" : ""}>{t('login.password')}</Label>
                <Input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value)
                    if(validationErrors.password) setValidationErrors(prev => ({...prev, password: null}))
                  }}
                  className={validationErrors.password ? "border-destructive focus-visible:ring-destructive" : ""}
                  placeholder="••••••••"
                />
                {validationErrors.password && (
                   <p className="text-xs text-destructive font-medium">{validationErrors.password}</p>
                )}
              </div>
              
              <Button type="submit" className="w-full" disabled={isLoading}>
                {isLoading ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    {t('login.loggingIn')}
                  </>
                ) : (
                  <>
                    {t('login.signIn')}
                    <ArrowRight className="ml-2 h-4 w-4" />
                  </>
                )}
              </Button>
            </form>
          </CardContent>
        </Card>
        
        <div className="mt-8 flex items-center justify-between px-2 text-xs text-muted-foreground font-bold uppercase tracking-widest">
           <p>{t('login.platformVersion')} 2.8</p>
           <Link to="/" className="hover:text-primary transition-colors">{t('login.support')}</Link>
        </div>
      </div>
    </div>
  )
}

export default Login