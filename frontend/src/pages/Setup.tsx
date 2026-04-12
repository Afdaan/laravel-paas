// ===========================================
// System Setup Page
// ===========================================
// First-time admin registration (Styled with shadcn)
// ===========================================

import React, { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { ShieldCheck, ArrowRight, Loader2, User, Mail, Lock } from 'lucide-react'
import { systemAPI } from '../services/api'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Sun, Moon } from 'lucide-react'
import { useTheme } from '@/components/ThemeProvider'
import useTranslation from '../lib/useTranslation'

interface SetupProps {
  onComplete: () => void;
}

const Setup: React.FC<SetupProps> = ({ onComplete }) => {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { theme, setTheme } = useTheme()
  const [isLoading, setIsLoading] = useState(false)
  const [formData, setFormData] = useState({
    name: '',
    email: '',
    password: '',
    confirmPassword: '',
  })
  const [validationErrors, setValidationErrors] = useState<Record<string, string | null>>({})

  useEffect(() => {
    const checkStatus = async () => {
      try {
        const { data } = await systemAPI.getInitStatus()
        if (data.is_initialized) {
          onComplete()
          navigate('/login', { replace: true })
        }
      } catch (e) {
        // Continue to setup if check fails
      }
    }
    checkStatus()
  }, [navigate, onComplete])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    
    const errors: Record<string, string | null> = {}
    if (!formData.name.trim()) errors.name = 'Full name is required'
    if (!formData.email.trim()) errors.email = 'Email address is required'
    if (!formData.password) errors.password = 'Password is required'
    if (formData.password !== formData.confirmPassword) errors.confirmPassword = 'Passwords do not match'
    if (formData.password && formData.password.length < 8) errors.password = 'Password must be at least 8 characters'

    if (Object.keys(errors).length > 0) {
      setValidationErrors(errors)
      return
    }

    setIsLoading(true)
    try {
      await systemAPI.initialize({
        name: formData.name,
        email: formData.email,
        password: formData.password,
      })
      
      toast.success(t('admin.setupSuccess'))
      onComplete()
      navigate('/login', { replace: true })
    } catch (err: any) {
      const message = err.response?.data?.error || t('common.actionFailed')
      toast.error(message)
    } finally {
      setIsLoading(false)
    }
  }

  const isDark = theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)

  return (
    <div className="min-h-screen flex items-center justify-center p-6 bg-background text-foreground font-sans relative">
      {/* Floating Theme Toggle */}
      <div className="absolute top-6 right-6">
         <Button
            variant="outline"
            size="icon"
            onClick={() => setTheme(isDark ? 'light' : 'dark')}
            className="rounded-full shadow-sm"
         >
            {isDark ? <Sun className="w-4 h-4 translate-y-0" /> : <Moon className="w-4 h-4 translate-y-0" />}
         </Button>
      </div>
      <div className="w-full max-w-md">
        {/* Logo/Brand Header */}
        <div className="text-center mb-10">
          <div className="inline-flex items-center justify-center w-14 h-14 rounded-xl bg-primary text-primary-foreground mb-4 shadow-lg shadow-primary/20">
            <ShieldCheck className="w-8 h-8" />
          </div>
          <h1 className="text-3xl font-bold tracking-tight text-foreground mb-2">Initialize System</h1>
          <p className="text-muted-foreground">Setup your first administrator account</p>
        </div>

        <Card className="border-border/50">
          <CardHeader className="space-y-1">
            <CardTitle className="text-xl text-center">First Admin Account</CardTitle>
            <CardDescription className="text-center text-xs">This user will have full superadmin privileges</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-5">
              {/* Name Field */}
              <div className="space-y-2">
                <Label htmlFor="name" className={validationErrors.name ? "text-destructive" : ""}>Full Name</Label>
                <div className="relative group">
                  <User className="absolute left-3 top-3 h-4 w-4 text-muted-foreground group-focus-within:text-primary transition-colors" />
                  <Input
                    id="name"
                    required
                    className={`pl-10 ${validationErrors.name ? "border-destructive focus-visible:ring-destructive" : ""}`}
                    placeholder="Admin Name"
                    value={formData.name}
                    onChange={(e) => {
                      setFormData({ ...formData, name: e.target.value })
                      if(validationErrors.name) setValidationErrors(prev => ({...prev, name: null}))
                    }}
                  />
                </div>
                {validationErrors.name && (
                   <p className="text-xs text-destructive font-medium pl-1">{validationErrors.name}</p>
                )}
              </div>

              {/* Email Field */}
              <div className="space-y-2">
                <Label htmlFor="email" className={validationErrors.email ? "text-destructive" : ""}>Email Address</Label>
                <div className="relative group">
                  <Mail className="absolute left-3 top-3 h-4 w-4 text-muted-foreground group-focus-within:text-primary transition-colors" />
                  <Input
                    id="email"
                    type="email"
                    required
                    className={`pl-10 ${validationErrors.email ? "border-destructive focus-visible:ring-destructive" : ""}`}
                    placeholder="admin@example.com"
                    value={formData.email}
                    onChange={(e) => {
                      setFormData({ ...formData, email: e.target.value })
                      if(validationErrors.email) setValidationErrors(prev => ({...prev, email: null}))
                    }}
                  />
                </div>
                {validationErrors.email && (
                   <p className="text-xs text-destructive font-medium pl-1">{validationErrors.email}</p>
                )}
              </div>

              {/* Password Field */}
              <div className="space-y-2">
                <Label htmlFor="password" className={validationErrors.password ? "text-destructive" : ""}>Password</Label>
                <div className="relative group">
                  <Lock className="absolute left-3 top-3 h-4 w-4 text-muted-foreground group-focus-within:text-primary transition-colors" />
                  <Input
                    id="password"
                    type="password"
                    required
                    className={`pl-10 ${validationErrors.password ? "border-destructive focus-visible:ring-destructive" : ""}`}
                    placeholder="••••••••"
                    value={formData.password}
                    onChange={(e) => {
                      setFormData({ ...formData, password: e.target.value })
                      if(validationErrors.password) setValidationErrors(prev => ({...prev, password: null}))
                    }}
                  />
                </div>
                {validationErrors.password && (
                   <p className="text-xs text-destructive font-medium pl-1">{validationErrors.password}</p>
                )}
              </div>

              {/* Confirm Password Field */}
              <div className="space-y-2">
                <Label htmlFor="confirmPassword" className={validationErrors.confirmPassword ? "text-destructive" : ""}>Confirm Password</Label>
                <div className="relative group">
                  <ShieldCheck className="absolute left-3 top-3 h-4 w-4 text-muted-foreground group-focus-within:text-primary transition-colors" />
                  <Input
                    id="confirmPassword"
                    type="password"
                    required
                    className={`pl-10 ${validationErrors.confirmPassword ? "border-destructive focus-visible:ring-destructive" : ""}`}
                    placeholder="••••••••"
                    value={formData.confirmPassword}
                    onChange={(e) => {
                      setFormData({ ...formData, confirmPassword: e.target.value })
                      if(validationErrors.confirmPassword) setValidationErrors(prev => ({...prev, confirmPassword: null}))
                    }}
                  />
                </div>
                {validationErrors.confirmPassword && (
                   <p className="text-xs text-destructive font-medium pl-1">{validationErrors.confirmPassword}</p>
                )}
              </div>

              {/* Submit Button */}
              <Button
                type="submit"
                disabled={isLoading}
                className="w-full mt-2"
              >
                {isLoading ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Initializing
                  </>
                ) : (
                  <>
                    Initialize System
                    <ArrowRight className="ml-2 h-4 w-4" />
                  </>
                )}
              </Button>
            </form>
          </CardContent>
        </Card>

        <p className="text-center text-muted-foreground mt-8 text-xs">
          Secure initialization powered by Laravel PaaS
        </p>
      </div>
    </div>
  )
}

export default Setup
