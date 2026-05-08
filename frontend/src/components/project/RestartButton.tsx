import { RotateCcw } from 'lucide-react'
import { toast } from 'sonner'
import useTranslation from '../../lib/useTranslation'
import { projectsAPI } from '../../services/api'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useState } from 'react'
import ConfirmationModal from '../ConfirmationModal'
import { AxiosError } from 'axios'

interface RestartButtonProps {
  projectId: string
  status: string
  onStarted?: () => void
  onSuccess?: () => void
  onError?: (error: any) => void
  className?: string
  variant?: "default" | "destructive" | "outline" | "secondary" | "ghost" | "link"
  size?: "default" | "sm" | "lg" | "icon"
}

export function RestartButton({
  projectId,
  status,
  onStarted,
  onSuccess,
  onError,
  className,
  variant = "outline",
  size = "default"
}: RestartButtonProps) {
  const { t } = useTranslation()
  const [isConfirmOpen, setIsConfirmOpen] = useState(false)
  
  const deployLocked = status === 'queued' || status === 'pending' || status === 'building' || status === 'restarting'

  const handleRestart = async (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (!projectId) return
    
    if (deployLocked) {
      return // Can't restart while building
    }

    setIsConfirmOpen(true)
  }

  const confirmRestart = async () => {
    setIsConfirmOpen(false)
    if (onStarted) onStarted()

    try {
      await toast.promise(
        projectsAPI.restart(projectId),
        {
          loading: t('common.loading'),
          success: t('projectDetail.actions.restartStarted'),
          error: t('common.error'),
        }
      )
      if (onSuccess) onSuccess()
    } catch (error: unknown) {
      const axiosError = error as AxiosError
      if (axiosError?.response?.status === 404) {
        toast.error(t('projectDetail.messages.restartUnavailable'))
      }
      if (onError) onError(error)
    }
  }

  return (
    <>
      <Button
        variant={variant}
        size={size}
        onClick={handleRestart}
        disabled={deployLocked || status === 'stopped'}
        className={cn("gap-2", (deployLocked || status === 'stopped') && "opacity-40", className)}
        title={t('projectDetail.actions.restart')}
      >
        <RotateCcw className="w-4 h-4" />
        <span className={cn(size === 'icon' && 'sr-only')}>
          {t('projectDetail.actions.restart')}
        </span>
      </Button>

      <ConfirmationModal
        isOpen={isConfirmOpen}
        onClose={() => setIsConfirmOpen(false)}
        onConfirm={confirmRestart}
        title={t('projectDetail.messages.restartConfirm')}
        message={t('projectDetail.messages.restartDesc')}
        type="warning"
        confirmText={t('projectDetail.actions.restart')}
      />
    </>
  )
}
