import { RefreshCw } from 'lucide-react'
import { toast } from 'sonner'
import useTranslation from '../../lib/useTranslation'
import { projectsAPI } from '../../services/api'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useState } from 'react'
import ConfirmationModal from '../ConfirmationModal'

interface RedeployButtonProps {
  projectId: string
  status: string
  onStarted?: () => void
  onSuccess?: () => void
  onError?: (error: any) => void
  className?: string
  variant?: "default" | "destructive" | "outline" | "secondary" | "ghost" | "link"
  size?: "default" | "sm" | "lg" | "icon"
}

export function RedeployButton({
  projectId,
  status,
  onStarted,
  onSuccess,
  onError,
  className,
  variant = "outline",
  size = "default"
}: RedeployButtonProps) {
  const { t } = useTranslation()
  const [isConfirmOpen, setIsConfirmOpen] = useState(false)
  
  const deployLocked = status === 'queued' || status === 'pending' || status === 'building' || status === 'restarting'

  const handleRedeploy = async (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (!projectId) return
    
    if (deployLocked) {
      toast.message(t('projectDetail.messages.buildTitle'), {
        description: `${t('projectDetail.actions.redeploy')} (${t(`status.${status}`)})`,
      })
      return
    }

    setIsConfirmOpen(true)
  }

  const confirmRedeploy = async () => {
    setIsConfirmOpen(false)
    if (onStarted) onStarted()

    try {
      await toast.promise(
        projectsAPI.redeploy(projectId),
        {
          loading: t('common.loading'),
          success: t('projectDetail.actions.redeployStarted'),
          error: t('common.error'),
        }
      )
      if (onSuccess) onSuccess()
    } catch (error) {
      if (onError) onError(error)
    }
  }

  return (
    <>
      <Button
        variant={variant}
        size={size}
        onClick={handleRedeploy}
        disabled={deployLocked}
        className={cn("gap-2", deployLocked && "opacity-40", className)}
        title={
          deployLocked
            ? `${t('projectDetail.actions.redeploy')} (${t(`status.${status}`)})`
            : t('projectDetail.actions.redeploy')
        }
      >
        <RefreshCw className={cn("w-4 h-4", status === 'building' && "animate-spin")} />
        <span className={cn(size === 'icon' && 'sr-only')}>
          {t('projectDetail.actions.redeploy')}
        </span>
      </Button>

      <ConfirmationModal
        isOpen={isConfirmOpen}
        onClose={() => setIsConfirmOpen(false)}
        onConfirm={confirmRedeploy}
        title={t('projectDetail.messages.redeployConfirm')}
        message={t('projectDetail.messages.redeployDesc')}
        type="warning"
        confirmText={t('projectDetail.actions.redeploy')}
      />
    </>
  )
}
