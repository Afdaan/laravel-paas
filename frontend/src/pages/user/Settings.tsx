import React, { useState, useEffect } from 'react'
import { toast } from 'sonner'
import { authAPI, githubAPI } from '../../services/api'
import useAuthStore from '../../stores/authStore'
import useTranslation from '../../lib/useTranslation'
import {
  User as UserIcon,
  Settings,
  Loader2,
  ExternalLink,
  Lock
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

const Github = (props: React.SVGProps<SVGSVGElement>) => (
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={props.className}
    style={props.style}
    width={props.width || "1em"}
    height={props.height || "1em"}
    {...props}
  >
    <path d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.08-1.25-.27-2.48-1-3.5.28-1.15.28-2.35 0-3.5 0 0-1 0-3 1.5-2.64-.5-5.36-.5-8 0C6 2 5 2 5 2c-.3 1.15-.3 2.35 0 3.5A5.403 5.403 0 0 0 4 9c0 3.5 3 5.5 6 5.5-.39.49-.68 1.05-.85 1.65-.17.6-.22 1.23-.15 1.85v4" />
    <path d="M9 18c-4.51 2-5-2-7-2" />
  </svg>
)


export function UserSettings() {
  const { t } = useTranslation()
  const { user, fetchUser } = useAuthStore()

  const [activeTab, setActiveTab] = useState<'profile' | 'security' | 'integrations'>('profile')
  const [isLoading, setIsLoading] = useState(false)
  const [installations, setInstallations] = useState<any[]>([])
  const [isGithubLoading, setIsGithubLoading] = useState(false)

  // Profile Form State
  const [profileData, setProfileData] = useState({
    name: user?.name || '',
    email: user?.email || '',
  })

  // Security Form State
  const [securityData, setSecurityData] = useState({
    password: '',
    newPassword: '',
    confirmPassword: '',
  })

  useEffect(() => {
    if (user) {
      setProfileData({
        name: user.name,
        email: user.email,
      })
    }
  }, [user])

  const loadInstallations = async () => {
    setIsGithubLoading(true)
    try {
      const response = await githubAPI.listInstallations()
      setInstallations(response.data.data || [])
    } catch (err) {
      console.error('Failed to load GitHub installations', err)
    } finally {
      setIsGithubLoading(false)
    }
  }

  useEffect(() => {
    if (activeTab === 'integrations') {
      loadInstallations()
    }
  }, [activeTab])

  // Handle Profile Update
  const handleProfileSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!profileData.name.trim() || !profileData.email.trim()) {
      toast.error(t('common.validation.required', { field: 'Fields' }))
      return
    }

    setIsLoading(true)
    try {
      await authAPI.updateProfile({
        name: profileData.name,
        email: profileData.email,
      })
      toast.success(t('user.settings.profileUpdated'))
      await fetchUser()
    } catch (err: any) {
      toast.error(err.response?.data?.error || t('common.actionFailed'))
    } finally {
      setIsLoading(false)
    }
  }

  // Handle Password Change
  const handleSecuritySubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!securityData.password || !securityData.newPassword || !securityData.confirmPassword) {
      toast.error(t('common.validation.required', { field: 'Passwords' }))
      return
    }

    if (securityData.newPassword !== securityData.confirmPassword) {
      toast.error(t('common.validation.mismatch') || 'Passwords do not match')
      return
    }

    setIsLoading(true)
    try {
      await authAPI.updateProfile({
        name: profileData.name,
        email: profileData.email,
        password: securityData.newPassword,
      })
      toast.success(t('user.settings.passwordChanged'))
      setSecurityData({
        password: '',
        newPassword: '',
        confirmPassword: '',
      })
    } catch (err: any) {
      toast.error(err.response?.data?.error || t('common.actionFailed'))
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="max-w-4xl mx-auto space-y-10 pb-20 animate-in fade-in duration-500">
      {/* Settings Header */}
      <div className="space-y-2 border-b border-border/40 pb-6">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-primary/10 flex items-center justify-center text-primary border border-primary/20">
            <Settings className="w-5 h-5" />
          </div>
          <div>
            <h1 className="text-3xl font-extrabold tracking-tight">
              {t('user.settings.title').split(' ')[0]} <span className="text-primary italic">{t('user.settings.title').split(' ')[1]}</span>
            </h1>
            <p className="text-muted-foreground text-xs uppercase tracking-widest font-bold mt-1">
              {t('user.settings.desc')}
            </p>
          </div>
        </div>
      </div>

      {/* Tabs Layout */}
      <div className="flex flex-col md:flex-row gap-8">
        {/* Navigation Sidebar */}
        <div className="w-full md:w-64 shrink-0 space-y-1">
          <Button
            variant="ghost"
            onClick={() => setActiveTab('profile')}
            className={cn(
              "flex items-center justify-start gap-3 w-full px-4 py-3 rounded-lg text-sm font-bold transition-all duration-200 cursor-pointer text-left border h-auto hover:bg-muted/10",
              activeTab === 'profile'
                ? "bg-card text-primary shadow-sm border-border/40 font-extrabold hover:text-primary hover:bg-card"
                : "text-muted-foreground border-transparent hover:text-foreground"
            )}
            style={{ cursor: 'pointer' }}
          >
            <UserIcon className="w-4 h-4 shrink-0" />
            {t('user.settings.profile')}
          </Button>
          <Button
            variant="ghost"
            onClick={() => setActiveTab('security')}
            className={cn(
              "flex items-center justify-start gap-3 w-full px-4 py-3 rounded-lg text-sm font-bold transition-all duration-200 cursor-pointer text-left border h-auto hover:bg-muted/10",
              activeTab === 'security'
                ? "bg-card text-primary shadow-sm border-border/40 font-extrabold hover:text-primary hover:bg-card"
                : "text-muted-foreground border-transparent hover:text-foreground"
            )}
            style={{ cursor: 'pointer' }}
          >
            <Lock className="w-4 h-4 shrink-0" />
            {t('user.settings.security')}
          </Button>
          <Button
            variant="ghost"
            onClick={() => setActiveTab('integrations')}
            className={cn(
              "flex items-center justify-start gap-3 w-full px-4 py-3 rounded-lg text-sm font-bold transition-all duration-200 cursor-pointer text-left border h-auto hover:bg-muted/10",
              activeTab === 'integrations'
                ? "bg-card text-primary shadow-sm border-border/40 font-extrabold hover:text-primary hover:bg-card"
                : "text-muted-foreground border-transparent hover:text-foreground"
            )}
            style={{ cursor: 'pointer' }}
          >
            <Github className="w-4 h-4 shrink-0" />
            {t('user.settings.integrations')}
          </Button>
        </div>

        {/* Tab Content Panels */}
        <div className="flex-1 min-w-0 space-y-6">
          {/* Tab 1: Profile */}
          {activeTab === 'profile' && (
            <Card>
              <CardHeader>
                <CardTitle className="text-xl font-bold">{t('user.settings.profile')}</CardTitle>
                <CardDescription>Update your personal information and contact details.</CardDescription>
              </CardHeader>
              <CardContent>
                <form onSubmit={handleProfileSubmit} className="space-y-6">
                  {/* Visual Avatar Section */}
                  <div className="flex items-center gap-5 p-4 rounded-xl border bg-muted/10">
                    <Avatar className="h-16 w-16 shrink-0 border-2 border-border">
                      {user?.avatar_url && <AvatarImage src={user.avatar_url} alt={user.name} className="object-cover" />}
                      <AvatarFallback className="bg-primary/10 text-primary text-xl font-bold">
                        {user?.name ? user.name.substring(0, 2).toUpperCase() : 'US'}
                      </AvatarFallback>
                    </Avatar>
                    <div className="space-y-1">
                      <h4 className="font-bold text-sm leading-none">{user?.name}</h4>
                      <p className="text-xs text-muted-foreground">{user?.email}</p>
                      {user?.avatar_url ? (
                        <Badge variant="secondary" className="text-[9px] font-bold uppercase tracking-wider bg-emerald-500/10 text-emerald-500 border-emerald-500/20">
                          GitHub Avatar Active
                        </Badge>
                      ) : (
                        <span className="text-[10px] text-muted-foreground italic">No external avatar linked.</span>
                      )}
                    </div>
                  </div>

                  <div className="space-y-4">
                    <div className="grid gap-2">
                      <Label htmlFor="name">{t('newProject.displayName')}</Label>
                      <Input
                        id="name"
                        value={profileData.name}
                        onChange={(e) => setProfileData(p => ({ ...p, name: e.target.value }))}
                        placeholder="Your full name"
                        className="h-11 rounded-xl"
                        disabled={isLoading}
                      />
                    </div>

                    <div className="grid gap-2">
                      <Label htmlFor="email">Email Address</Label>
                      <Input
                        id="email"
                        type="email"
                        value={profileData.email}
                        onChange={(e) => setProfileData(p => ({ ...p, email: e.target.value }))}
                        placeholder="name@company.com"
                        className="h-11 rounded-xl"
                        disabled={isLoading}
                      />
                    </div>
                  </div>

                  <Button type="submit" disabled={isLoading} className="h-11 px-6 rounded-xl font-bold cursor-pointer" style={{ cursor: 'pointer' }}>
                    {isLoading && <Loader2 className="w-4 h-4 animate-spin mr-2" />}
                    {t('user.settings.updateProfile')}
                  </Button>
                </form>
              </CardContent>
            </Card>
          )}

          {/* Tab 2: Security */}
          {activeTab === 'security' && (
            <Card>
              <CardHeader>
                <CardTitle className="text-xl font-bold">{t('user.settings.changePassword')}</CardTitle>
                <CardDescription>Ensure your account remains safe using a strong, unique credential password.</CardDescription>
              </CardHeader>
              <CardContent>
                <form onSubmit={handleSecuritySubmit} className="space-y-6">
                  <div className="space-y-4">
                    <div className="grid gap-2">
                      <Label htmlFor="password">{t('user.settings.currentPassword')}</Label>
                      <Input
                        id="password"
                        type="password"
                        value={securityData.password}
                        onChange={(e) => setSecurityData(p => ({ ...p, password: e.target.value }))}
                        placeholder="••••••••"
                        className="h-11 rounded-xl"
                        disabled={isLoading}
                      />
                    </div>

                    <div className="grid gap-2">
                      <Label htmlFor="newPassword">{t('user.settings.newPassword')}</Label>
                      <Input
                        id="newPassword"
                        type="password"
                        value={securityData.newPassword}
                        onChange={(e) => setSecurityData(p => ({ ...p, newPassword: e.target.value }))}
                        placeholder="••••••••"
                        className="h-11 rounded-xl"
                        disabled={isLoading}
                      />
                    </div>

                    <div className="grid gap-2">
                      <Label htmlFor="confirmPassword">{t('user.settings.confirmNewPassword')}</Label>
                      <Input
                        id="confirmPassword"
                        type="password"
                        value={securityData.confirmPassword}
                        onChange={(e) => setSecurityData(p => ({ ...p, confirmPassword: e.target.value }))}
                        placeholder="••••••••"
                        className="h-11 rounded-xl"
                        disabled={isLoading}
                      />
                    </div>
                  </div>

                  <Button type="submit" disabled={isLoading} className="h-11 px-6 rounded-xl font-bold cursor-pointer" style={{ cursor: 'pointer' }}>
                    {isLoading && <Loader2 className="w-4 h-4 animate-spin mr-2" />}
                    {t('user.settings.changePassword')}
                  </Button>
                </form>
              </CardContent>
            </Card>
          )}

          {/* Tab 3: Integrations */}
          {activeTab === 'integrations' && (
            <Card>
              <CardHeader>
                <CardTitle className="text-xl font-bold flex items-center gap-2">
                  <Github className="w-5 h-5 text-foreground" />
                  {t('user.settings.github')}
                </CardTitle>
                <CardDescription>
                  {t('user.settings.githubDesc')}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-8">
                {/* Connection Status Badge */}
                <div className="flex items-center justify-between p-5 rounded-xl border bg-muted/10">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-lg bg-foreground/10 flex items-center justify-center text-foreground border">
                      <Github className="w-5 h-5" />
                    </div>
                    <div>
                      <h4 className="font-bold text-sm leading-none">GitHub Developer Access</h4>
                      <p className="text-xs text-muted-foreground mt-1">Status: {installations.length > 0 ? t('user.settings.connected') : t('user.settings.disconnected')}</p>
                    </div>
                  </div>

                  <Badge variant="outline" className={cn(
                    "text-[10px] uppercase font-bold tracking-widest px-2.5 py-1",
                    installations.length > 0 
                      ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/20" 
                      : "bg-amber-500/10 text-amber-500 border-amber-500/20"
                  )}>
                    {installations.length > 0 ? t('user.settings.connected') : t('user.settings.disconnected')}
                  </Badge>
                </div>

                {/* installations display or Action button */}
                {isGithubLoading ? (
                  <div className="flex flex-col items-center justify-center py-10 gap-3 text-muted-foreground">
                    <Loader2 className="w-8 h-8 animate-spin text-primary" />
                    <p className="text-xs">Loading connected installations...</p>
                  </div>
                ) : installations.length > 0 ? (
                  <div className="space-y-4">
                    <h4 className="text-xs font-bold uppercase tracking-widest text-muted-foreground">{t('user.settings.authorizedAccounts')}</h4>
                    <div className="space-y-3">
                      {installations.map((inst) => (
                        <div key={inst.installation_id} className="flex items-center justify-between p-4 rounded-xl border bg-card/45 hover:border-primary/20 transition-all">
                          <div className="flex items-center gap-3">
                            {inst.avatar_url ? (
                              <img src={inst.avatar_url} alt={inst.account_name} className="w-8 h-8 rounded-full border" />
                            ) : (
                              <div className="w-8 h-8 rounded-full bg-muted flex items-center justify-center text-muted-foreground">
                                <UserIcon className="w-4 h-4" />
                              </div>
                            )}
                            <div>
                              <h5 className="font-bold text-sm leading-none">{inst.account_name}</h5>
                              <p className="text-[10px] text-muted-foreground font-mono mt-1">ID: {inst.installation_id}</p>
                            </div>
                          </div>

                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => {
                              const appUrl = import.meta.env.VITE_GITHUB_APP_URL || 'https://github.com/apps/laravel-paas-local'
                              window.open(`${appUrl}/installations/new`, '_blank')
                            }}
                            className="gap-2 h-9 rounded-lg"
                          >
                            Configure
                            <ExternalLink className="w-3 h-3" />
                          </Button>
                        </div>
                      ))}
                    </div>
                  </div>
                ) : (
                  <div className="border border-dashed border-border rounded-xl p-8 text-center space-y-4 bg-muted/5">
                    <div className="w-12 h-12 rounded-xl bg-muted flex items-center justify-center mx-auto text-muted-foreground">
                      <Github className="w-6 h-6" />
                    </div>
                    <div className="space-y-1">
                      <h4 className="font-bold text-base">{t('newProject.connectGithub')}</h4>
                      <p className="text-sm text-muted-foreground max-w-md mx-auto">
                        {t('newProject.connectGithubDesc')}
                      </p>
                    </div>
                    <Button
                      onClick={() => {
                        const appUrl = import.meta.env.VITE_GITHUB_APP_URL || 'https://github.com/apps/laravel-paas-local'
                        window.open(`${appUrl}/installations/new`, '_blank')
                      }}
                      className="gap-2 mx-auto h-11 rounded-xl cursor-pointer"
                      style={{ cursor: 'pointer' }}
                    >
                      <Github className="w-4 h-4" />
                      {t('newProject.configureGithubApp')}
                      <ExternalLink className="w-3.5 h-3.5" />
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}
