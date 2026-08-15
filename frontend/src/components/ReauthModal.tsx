import { useState, useEffect, FormEvent, useRef } from 'react'
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
  const isSubmittingRef = useRef(false)
  const challengeOriginRef = useRef<string | null>(null)
  const currentLocationRef = useRef(location.pathname)
  currentLocationRef.current = location.pathname

  // Automatically close modal if user logs out, token expires, is on login/setup page,
  // or navigates away from the challenge origin page
  useEffect(() => {
    if (!token || location.pathname === '/login' || location.pathname === '/setup') {
      if (open) {
        setOpen(false)
        setPassword('')
        setError('')
        challengeOriginRef.current = null
        window.dispatchEvent(new Event('auth:reauth_cancelled'))
      }
      return
    }

    if (open && challengeOriginRef.current && location.pathname !== challengeOriginRef.current) {
      setOpen(false)
      setPassword('')
      setError('')
      challengeOriginRef.current = null
      window.dispatchEvent(new Event('auth:reauth_cancelled'))
    }
  }, [token, location.pathname, open])

  useEffect(() => {
    const handleRecentAuthRequired = () => {
      const currentPath = currentLocationRef.current
      // Don't show re-auth modal if user is on login/setup or unauthenticated
      if (!useAuthStore.getState().token || currentPath === '/login' || currentPath === '/setup') {
        return
      }
      if (!challengeOriginRef.current) {
        challengeOriginRef.current = currentPath
      }
      setError('')
      setOpen(true)
    }

    const handleAuthExpired = () => {
      setOpen(false)
      setPassword('')
      setError('')
      challengeOriginRef.current = null
      window.dispatchEvent(new Event('auth:reauth_cancelled'))
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
    if (!password || isSubmittingRef.current) return

    isSubmittingRef.current = true
    setLoading(true)
    setError('')

    try {
      await authAPI.reauthenticate(password)
      setOpen(false)
      setPassword('')
      challengeOriginRef.current = null
      window.dispatchEvent(new Event('auth:reauthenticated'))
    } catch (err: unknown) {
      const resp = (err as { response?: { status?: number; data?: { message?: string; code?: string; error?: string } } })?.response
      const code = resp?.data?.code

      // If wrong password / credential check failed on re-auth endpoint, keep modal open and display error
      if (resp?.status === 401 && (code === 'AUTH_FAILED' || code === 'INVALID_CREDENTIALS')) {
        setError(resp?.data?.error || resp?.data?.message || t('common.reauthFailed'))
        return
      }

      // If genuine session expiration (TOKEN_INVALID, TOKEN_REVOKED, UNAUTHORIZED, etc.)
      if (resp?.status === 401) {
        setOpen(false)
        setPassword('')
        challengeOriginRef.current = null
        window.dispatchEvent(new Event('auth:reauth_cancelled'))
        window.dispatchEvent(new Event('auth:expired'))
        return
      }
      setError(resp?.data?.message || resp?.data?.error || t('common.reauthFailed'))
    } finally {
      setLoading(false)
      isSubmittingRef.current = false
    }
  }

  const handleOpenChange = (isOpen: boolean) => {
    setOpen(isOpen)
    if (!isOpen) {
      setPassword('')
      setError('')
      challengeOriginRef.current = null
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
