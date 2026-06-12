import { ChevronDown, RefreshCw, Sparkles, Zap } from 'lucide-react'
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
  DropdownMenuSeparator,
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
        "inline-flex items-center rounded-lg bg-background/95 shadow-sm shadow-black/5 ring-1 ring-border/80 transition-colors dark:bg-input/20 dark:ring-white/10",
        variant !== "ghost" && "border border-input",
        className
      )}>
        <Button
          variant="ghost"
          size={size}
          onClick={(e) => handleRedeploy(e, false)}
          disabled={deployLocked}
          className={cn(
            "h-9 rounded-none rounded-l-lg border-r border-border/80 px-3 gap-2 hover:bg-muted/80 focus-visible:ring-0 focus-visible:ring-offset-0 dark:border-white/10",
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
              "h-9 w-8 rounded-none rounded-r-lg px-0 cursor-pointer hover:bg-muted/80 focus-visible:ring-0 focus-visible:ring-offset-0 data-[popup-open]:bg-muted/80",
              deployLocked && "opacity-40"
            )}
            aria-label={t('projectDetail.actions.redeploy')}
          >
            <ChevronDown className="h-4 w-4 opacity-70 transition-transform duration-200 group-aria-expanded/button:rotate-180" />
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="end"
            sideOffset={8}
            className="w-[246px] rounded-xl border border-border/70 bg-popover/95 p-1.5 shadow-xl shadow-black/10 ring-1 ring-white/10 backdrop-blur-xl dark:border-white/10 dark:shadow-black/40"
          >
            <DropdownMenuItem 
              onClick={(e) => {
                const mouseEvent = e as unknown as React.MouseEvent;
                handleRedeploy(mouseEvent, false);
              }}
              className="cursor-pointer items-start gap-3 rounded-lg px-2.5 py-2.5 transition-colors focus:bg-primary/10 focus:text-foreground"
            >
              <span className="mt-0.5 flex size-8 items-center justify-center rounded-md border border-emerald-500/20 bg-emerald-500/10 text-emerald-500">
                <Zap className="size-4" />
              </span>
              <span className="flex min-w-0 flex-col">
                <span className="text-sm font-semibold leading-none text-foreground">
                  {t('projectDetail.actions.redeployFast') || 'Redeploy (Cached)'}
                </span>
                <span className="mt-1 text-xs leading-4 text-muted-foreground">
                  {t('projectDetail.actions.redeployFastDesc') || 'Reuse build cache for a faster deploy'}
                </span>
              </span>
            </DropdownMenuItem>
            <DropdownMenuSeparator className="mx-2 my-1 bg-border/70" />
            <DropdownMenuItem 
              onClick={(e) => {
                const mouseEvent = e as unknown as React.MouseEvent;
                handleRedeploy(mouseEvent, true);
              }}
              className="cursor-pointer items-start gap-3 rounded-lg px-2.5 py-2.5 transition-colors focus:bg-amber-500/10 focus:text-foreground"
            >
              <span className="mt-0.5 flex size-8 items-center justify-center rounded-md border border-amber-500/25 bg-amber-500/10 text-amber-500">
                <Sparkles className="size-4" />
              </span>
              <span className="flex min-w-0 flex-col">
                <span className="text-sm font-semibold leading-none text-foreground">
                  {t('projectDetail.actions.redeployClean') || 'Clean Rebuild'}
                </span>
                <span className="mt-1 text-xs leading-4 text-muted-foreground">
                  {t('projectDetail.actions.redeployCleanDesc') || 'Bypass cache and rebuild from scratch'}
                </span>
              </span>
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
