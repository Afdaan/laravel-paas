import { useState, useEffect, FormEvent } from 'react'
import { useLocation } from 'react-router-dom'
import { authAPI } from '@/services/api'
import useTranslation from '@/lib/useTranslation'
import useAuthStore from '@/stores/authStore'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { ShieldCheck, Loader2 } from 'lucide-react'

export default function ReauthModal() {
  const { t } = useTranslation()
  const location = useLocation()
  const { user, token } = useAuthStore()
  const [open, setOpen] = useState(false)
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  // Automatically close modal if user logs out, token expires, or is on login page
  useEffect(() => {
    if (!token || location.pathname === '/login' || location.pathname === '/setup') {
      setOpen(false)
      setPassword('')
      setError('')
    }
  }, [token, location.pathname])

  useEffect(() => {
    const handleRecentAuthRequired = () => {
      // Don't show re-auth modal if user is on login/setup or unauthenticated
      if (!useAuthStore.getState().token || window.location.pathname === '/login') {
        return
      }
      setPassword('')
      setError('')
      setOpen(true)
    }

    const handleAuthExpired = () => {
      setOpen(false)
      setPassword('')
      setError('')
    }

    window.addEventListener('auth:recent_auth_required', handleRecentAuthRequired)
    window.addEventListener('auth:expired', handleAuthExpired)
    return () => {
      window.removeEventListener('auth:recent_auth_required', handleRecentAuthRequired)
      window.removeEventListener('auth:expired', handleAuthExpired)
    }
  }, [])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!password) return

    setLoading(true)
    setError('')

    try {
      await authAPI.reauthenticate(password)
      setOpen(false)
      setPassword('')
      window.dispatchEvent(new Event('auth:reauthenticated'))
    } catch (err: unknown) {
      const resp = (err as { response?: { status?: number; data?: { message?: string; code?: string } } })?.response
      // If 401 Unauthorized or session expired on re-auth endpoint, close modal and let global auth handler handle redirect
      if (resp?.status === 401) {
        setOpen(false)
        setPassword('')
        window.dispatchEvent(new Event('auth:expired'))
        return
      }
      setError(resp?.data?.message || t('common.reauthFailed'))
    } finally {
      setLoading(false)
    }
  }

  const handleOpenChange = (isOpen: boolean) => {
    setOpen(isOpen)
    if (!isOpen) {
      setPassword('')
      setError('')
      window.dispatchEvent(new Event('auth:reauth_cancelled'))
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent showCloseButton={false} className="sm:max-w-md">
        <DialogHeader>
          <div className="flex items-center gap-2 text-primary">
            <ShieldCheck className="size-5 shrink-0" />
            <DialogTitle>{t('common.reauthTitle')}</DialogTitle>
          </div>
          <DialogDescription>
            {t('common.reauthDesc')}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Hidden username input to enable browser password manager / 1Password / Bitwarden autofill */}
          <input
            type="text"
            name="username"
            value={user?.email || ''}
            readOnly
            tabIndex={-1}
            autoComplete="username"
            aria-hidden="true"
            className="sr-only"
          />

          <div className="space-y-2">
            <Label htmlFor="reauth-password">{t('common.reauthPasswordLabel')}</Label>
            <Input
              id="reauth-password"
              name="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              autoComplete="current-password"
              autoFocus
              required
            />
            {error && (
              <p className="text-xs font-medium text-destructive">{error}</p>
            )}
          </div>

          <DialogFooter showCloseButton={false}>
            <Button
              type="button"
              variant="outline"
              onClick={() => handleOpenChange(false)}
              disabled={loading}
            >
              {t('common.reauthCancel')}
            </Button>
            <Button type="submit" disabled={loading || !password}>
              {loading && <Loader2 className="mr-2 size-4 animate-spin" />}
              {t('common.reauthSubmit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
