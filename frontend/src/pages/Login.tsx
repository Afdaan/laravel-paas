import React, { useState, useEffect } from 'react'
import { useNavigate, Link, useLocation } from 'react-router-dom'
import { toast } from 'sonner'
import { systemAPI } from '../services/api'
import { AxiosError } from 'axios'
import useAuthStore from '../stores/authStore'
import useTranslation from '../lib/useTranslation'
import { ArrowRight, ArrowLeft, Terminal, Loader2, Eye, EyeOff, Sun, Moon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useTheme } from '@/components/ThemeProvider'

function Login() {
  const { t } = useTranslation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [validationErrors, setValidationErrors] = useState<Record<string, string | null>>({})
  
  const login = useAuthStore((state) => state.login)
  const token = useAuthStore((state) => state.token)
  const user = useAuthStore((state) => state.user)
  const { theme, setTheme } = useTheme()
  const navigate = useNavigate()
  const location = useLocation()
  
  // Get redirect path from state, default to dashboards
  const from = location.state?.from?.pathname || null

  useEffect(() => {
    // Check if system is initialized
    const checkInit = async () => {
      try {
        const { data } = await systemAPI.getInitStatus()
        if (!data.is_initialized) {
          navigate('/setup', { replace: true })
        }
      } catch (e) {
        // Ignore errors during initial check, will be caught by global offline handler
      }
    }
    checkInit()

    if (token && user) {
      const isAdmin = user.role === 'superadmin' || user.role === 'admin'
      const destination = from || (isAdmin ? '/admin/dashboard' : '/dashboard')
      navigate(destination, { replace: true })
    }
  }, [token, user, navigate, from])
  
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
      
      const isAdminMatched = user.role === 'superadmin' || user.role === 'admin'
      const destination = from || (isAdminMatched ? '/admin/dashboard' : '/dashboard')
      navigate(destination)
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: string }>
      toast.error(axiosError.response?.data?.error || t('login.failed'))
    } finally {
      setIsLoading(false)
    }
  }
  
  const isDark = theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)

  return (
    <div className="relative flex min-h-screen items-center justify-center bg-background px-4 py-10 text-sm text-foreground sm:px-6">
      <div className="absolute right-4 top-4 sm:right-6 sm:top-6">
        <Button
          type="button"
          variant="outline"
          size="icon-sm"
          onClick={() => setTheme(isDark ? 'light' : 'dark')}
          className="rounded-full"
          title={t('common.theme')}
        >
          {isDark ? <Sun className="size-3.5" /> : <Moon className="size-3.5" />}
        </Button>
      </div>

      <div className="w-full max-w-[420px]">
        <Button
          variant="ghost"
          size="sm"
          className="mb-6 text-muted-foreground"
          render={<Link to="/" className="group" />}
          nativeButton={false}
        >
          <ArrowLeft className="size-3.5 transition-transform group-hover:-translate-x-0.5" />
          {t('login.backToHome')}
        </Button>
        
        <Card size="sm" className="border-border/60 shadow-sm shadow-foreground/5">
          <CardHeader className="justify-items-center gap-3 px-6 pt-6 pb-2 text-center">
            <div className="flex size-11 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-sm">
              <Terminal className="size-5" />
            </div>
            <div className="space-y-1 text-center">
              <CardTitle className="text-xl">{t('login.signIn')}</CardTitle>
              <CardDescription className="max-w-xs leading-5">{t('login.desc')}</CardDescription>
            </div>
          </CardHeader>
          <CardContent className="px-6 pb-6">
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="email" className={validationErrors.email ? 'text-destructive' : 'text-foreground'}>
                  {t('login.email')}
                </Label>
                <Input
                  id="email"
                  type="email"
                  value={email}
                  onChange={(e) => {
                    setEmail(e.target.value)
                    if (validationErrors.email) setValidationErrors((prev) => ({ ...prev, email: null }))
                  }}
                  aria-invalid={!!validationErrors.email}
                  className="h-9 bg-muted/30"
                  placeholder={t('login.emailPlaceholder')}
                  autoComplete="email"
                  autoFocus
                />
                {validationErrors.email && (
                   <p className="text-xs text-destructive font-medium">{validationErrors.email}</p>
                )}
              </div>
              
              <div className="space-y-2">
                <Label htmlFor="password" className={validationErrors.password ? 'text-destructive' : 'text-foreground'}>
                  {t('login.password')}
                </Label>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? 'text' : 'password'}
                    value={password}
                    onChange={(e) => {
                      setPassword(e.target.value)
                      if (validationErrors.password) setValidationErrors((prev) => ({ ...prev, password: null }))
                    }}
                    aria-invalid={!!validationErrors.password}
                    className="h-9 bg-muted/30 pr-10"
                    placeholder={t('login.passwordPlaceholder')}
                    autoComplete="current-password"
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                      setShowPassword((prev) => !prev)
                    }}
                    className="absolute right-1 top-1 text-muted-foreground hover:text-foreground"
                    aria-label={showPassword ? 'Hide password' : 'Show password'}
                  >
                    {showPassword ? (
                      <EyeOff className="size-3.5" />
                    ) : (
                      <Eye className="size-3.5" />
                    )}
                  </Button>
                </div>
                {validationErrors.password && (
                   <p className="text-xs text-destructive font-medium">{validationErrors.password}</p>
                )}
              </div>
              
              <Button type="submit" size="lg" className="w-full" disabled={isLoading}>
                {isLoading ? (
                  <>
                    <Loader2 className="size-4 animate-spin" />
                    {t('login.loggingIn')}
                  </>
                ) : (
                  <>
                    {t('login.signIn')}
                    <ArrowRight className="size-4" />
                  </>
                )}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

export default Login
