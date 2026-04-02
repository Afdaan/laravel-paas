import React, { useState, useEffect } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { toast } from 'sonner'
import useAuthStore from '../stores/authStore'
import { ArrowRight, ArrowLeft, Terminal, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

function Login() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [validationErrors, setValidationErrors] = useState<Record<string, string | null>>({})
  
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
  
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    
    const errors: Record<string, string | null> = {}
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
      
      if (user.role === 'superadmin' || user.role === 'admin') {
        navigate('/admin/dashboard')
      } else {
        navigate('/dashboard')
      }
    } catch (error: any) {
      toast.error(error.response?.data?.error || 'Login failed')
    } finally {
      setIsLoading(false)
    }
  }
  
  return (
    <div className="min-h-screen flex items-center justify-center p-6 bg-background text-foreground font-sans">
      <div className="w-full max-w-md">
        <Button variant="ghost" render={<Link to="/" className="text-muted-foreground group" />} className="mb-8">
            <ArrowLeft className="w-4 h-4 mr-2 group-hover:-translate-x-1 transition-transform" />
            Back to Home
        </Button>
        
        <Card>
          <CardHeader className="space-y-1">
             <div className="flex items-center justify-center mb-4">
                 <div className="w-12 h-12 bg-primary text-primary-foreground rounded-lg flex items-center justify-center flex-shrink-0">
                    <Terminal className="w-6 h-6" />
                 </div>
             </div>
             <CardTitle className="text-2xl text-center">Sign In</CardTitle>
             <CardDescription className="text-center">Enter your credentials to access your account.</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="email" className={validationErrors.email ? "text-destructive" : ""}>Email Address</Label>
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
                <Label htmlFor="password" className={validationErrors.password ? "text-destructive" : ""}>Password</Label>
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
                    Logging In
                  </>
                ) : (
                  <>
                    Sign In
                    <ArrowRight className="ml-2 h-4 w-4" />
                  </>
                )}
              </Button>
            </form>
          </CardContent>
        </Card>
        
        <div className="mt-8 flex items-center justify-between px-2 text-xs text-muted-foreground">
           <p>Platform Version 2.8</p>
           <Link to="/" className="hover:text-primary transition-colors">Support</Link>
        </div>
      </div>
    </div>
  )
}

export default Login