import React, { useState, useEffect } from 'react'
import { useNavigate, Link, useLocation } from 'react-router-dom'
import { toast } from 'sonner'
import { systemAPI } from '../services/api'
import { AxiosError } from 'axios'
import useAuthStore from '../stores/authStore'
import useTranslation from '../lib/useTranslation'
import { ArrowRight, ArrowLeft, Eye, EyeOff, Sun, Moon } from 'lucide-react'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useTheme } from '@/components/ThemeProvider'

function Login() {
  const { t, language } = useTranslation()
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

  useEffect(() => {
    document.documentElement.lang = language
  }, [language])

  return (
    <div className={`${isDark ? 'runara-landing' : ''} h-full overflow-x-hidden overflow-y-auto bg-background font-sans text-foreground antialiased`}>
      <header className="border-b border-border bg-background">
        <nav className="mx-auto flex min-h-20 w-full max-w-[90rem] items-center justify-between gap-4 px-4 sm:px-6 lg:px-10" aria-label={t('landing.primaryNavigation')}>
          <Link to="/" className="flex min-h-11 items-center gap-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
            <img src="/runara-icon.png" alt="" className="size-9 object-contain" />
            <span className="text-base font-extrabold tracking-[-0.03em]">Runara</span>
          </Link>
          <div className={isDark ? 'runara-surface flex items-center gap-2' : 'flex items-center gap-2'}>
            <LanguageSwitcher className="size-11 rounded-sm border" forceDark={isDark} />
            <Button
              type="button"
              variant="outline"
              size="icon"
              onClick={() => setTheme(isDark ? 'light' : 'dark')}
              className="size-11 rounded-sm"
              aria-label={t('common.theme')}
              title={t('common.theme')}
            >
              {isDark ? <Sun className="size-4" aria-hidden="true" /> : <Moon className="size-4" aria-hidden="true" />}
            </Button>
          </div>
        </nav>
      </header>

      <main className="flex min-h-[calc(100%_-_5rem)] items-center px-4 py-10 sm:px-6 sm:py-16 lg:px-10">
        <div className="mx-auto w-full max-w-lg">
          <Button
            variant="ghost"
            size="sm"
            className="mb-6 min-h-11 px-0 text-muted-foreground hover:bg-transparent hover:text-foreground"
            render={<Link to="/" />}
            nativeButton={false}
          >
            <ArrowLeft className="size-4 shrink-0" aria-hidden="true" />
            {t('login.backToHome')}
          </Button>

          <section className="border border-border bg-card" aria-labelledby="login-title">
            <header className="border-b border-border px-5 py-6 sm:px-8 sm:py-8">
              <h1 id="login-title" className="text-3xl font-bold tracking-[-0.04em] sm:text-4xl">{t('login.signIn')}</h1>
              <p className="mt-3 max-w-sm leading-6 text-muted-foreground">{t('login.desc')}</p>
            </header>
            <div className="px-5 py-6 sm:px-8 sm:py-8">
              <form onSubmit={handleSubmit} className="space-y-5" noValidate>
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
                    aria-describedby={validationErrors.email ? 'email-error' : undefined}
                    className="h-11 border-border bg-background"
                    placeholder={t('login.emailPlaceholder')}
                    autoComplete="email"
                    autoFocus
                  />
                  {validationErrors.email && (
                    <p id="email-error" role="alert" className="text-xs font-medium text-destructive">{validationErrors.email}</p>
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
                      aria-describedby={validationErrors.password ? 'password-error' : undefined}
                      className="h-11 border-border bg-background pr-12"
                      placeholder={t('login.passwordPlaceholder')}
                      autoComplete="current-password"
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      onClick={(e) => {
                        e.preventDefault()
                        e.stopPropagation()
                        setShowPassword((prev) => !prev)
                      }}
                      className="absolute right-0 top-0 size-11 text-muted-foreground hover:text-foreground"
                      aria-label={showPassword ? t('login.hidePassword') : t('login.showPassword')}
                      aria-pressed={showPassword}
                    >
                      {showPassword ? <EyeOff className="size-4" aria-hidden="true" /> : <Eye className="size-4" aria-hidden="true" />}
                    </Button>
                  </div>
                  {validationErrors.password && (
                    <p id="password-error" role="alert" className="text-xs font-medium text-destructive">{validationErrors.password}</p>
                  )}
                </div>

                <Button type="submit" size="lg" className="min-h-12 w-full font-bold" disabled={isLoading}>
                  {isLoading ? (
                    <>
                      <Spinner className="size-4" />
                      {t('login.loggingIn')}
                    </>
                  ) : (
                    <>
                      {t('login.signIn')}
                      <ArrowRight className="size-4" aria-hidden="true" />
                    </>
                  )}
                </Button>
              </form>
            </div>
          </section>
        </div>
      </main>
    </div>
  )
}

export default Login
