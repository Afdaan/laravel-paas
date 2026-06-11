import { RefreshCw, ChevronDown } from 'lucide-react'
import { toast } from 'sonner'
import useTranslation from '../../lib/useTranslation'
import { projectsAPI } from '../../services/api'
import { Button, buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useState } from 'react'
import ConfirmationModal from '../ConfirmationModal'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

interface RedeployButtonProps {
  projectId: string
  status: string
  deploymentStatus?: string
  onStarted?: () => void
  onSuccess?: () => void
  onError?: (error: unknown) => void
  className?: string
  variant?: "default" | "destructive" | "outline" | "secondary" | "ghost" | "link"
  size?: "default" | "sm" | "lg" | "icon"
}

export function RedeployButton({
  projectId,
  status,
  deploymentStatus,
  onStarted,
  onSuccess,
  onError,
  className,
  variant = "outline",
  size = "default"
}: RedeployButtonProps) {
  const { t } = useTranslation()
  const [isConfirmOpen, setIsConfirmOpen] = useState(false)
  const [isClean, setIsClean] = useState(false)
  
  const isCurrentlyDeploying = Boolean(deploymentStatus && !['completed', 'failed', 'rollback', 'cancelled'].includes(deploymentStatus))
  const deployLocked = isCurrentlyDeploying || status === 'queued' || status === 'pending' || status === 'building' || status === 'restarting'

  const handleRedeploy = async (e: React.MouseEvent, clean: boolean = false) => {
    e.preventDefault()
    e.stopPropagation()
    if (!projectId) return
    
    if (deployLocked) {
      toast.message(t('projectDetail.messages.buildTitle'), {
        description: `${t('projectDetail.actions.redeploy')} (${t(`status.${status}`)})`,
      })
      return
    }

    setIsClean(clean)
    setIsConfirmOpen(true)
  }

  const confirmRedeploy = async () => {
    setIsConfirmOpen(false)
    if (onStarted) onStarted()

    try {
      await toast.promise(
        projectsAPI.redeploy(projectId, isClean),
        {
          loading: t('common.loading'),
          success: isClean ? t('projectDetail.actions.redeployCleanStarted') : t('projectDetail.actions.redeployStarted'),
          error: t('common.error'),
        }
      )
      if (onSuccess) onSuccess()
    } catch (error: unknown) {
      if (onError) onError(error)
    }
  }

  return (
    <>
      <div className={cn(
        "inline-flex items-center rounded-md bg-background shadow-sm",
        variant !== "ghost" && "border border-input",
        className
      )}>
        <Button
          variant="ghost"
          size={size}
          onClick={(e) => handleRedeploy(e, false)}
          disabled={deployLocked}
          className={cn(
            "h-9 px-3 rounded-none rounded-l-md border-r border-input gap-2 focus-visible:ring-0 focus-visible:ring-offset-0", 
            deployLocked && "opacity-40"
          )}
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

        <DropdownMenu>
          <DropdownMenuTrigger
            disabled={deployLocked}
            className={cn(
              buttonVariants({ variant: "ghost", size: "icon" }),
              "h-9 w-8 px-0 rounded-none rounded-r-md focus-visible:ring-0 focus-visible:ring-offset-0 cursor-pointer",
              deployLocked && "opacity-40"
            )}
          >
            <ChevronDown className="h-4 w-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-[200px]">
            <DropdownMenuItem 
              onClick={(e) => {
                const mouseEvent = e as unknown as React.MouseEvent;
                handleRedeploy(mouseEvent, false);
              }}
              className="cursor-pointer"
            >
              {t('projectDetail.actions.redeployFast') || 'Redeploy (Cached)'}
            </DropdownMenuItem>
            <DropdownMenuItem 
              onClick={(e) => {
                const mouseEvent = e as unknown as React.MouseEvent;
                handleRedeploy(mouseEvent, true);
              }}
              className="text-amber-600 focus:text-amber-600 cursor-pointer"
            >
              {t('projectDetail.actions.redeployClean') || 'Clean Rebuild (No Cache)'}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <ConfirmationModal
        isOpen={isConfirmOpen}
        onClose={() => setIsConfirmOpen(false)}
        onConfirm={confirmRedeploy}
        title={isClean ? t('projectDetail.messages.redeployCleanConfirm') : t('projectDetail.messages.redeployConfirm')}
        message={isClean ? t('projectDetail.messages.redeployCleanDesc') : t('projectDetail.messages.redeployDesc')}
        type="warning"
        confirmText={isClean ? t('projectDetail.actions.redeployClean') : t('projectDetail.actions.redeploy')}
      />
    </>
  )
}
