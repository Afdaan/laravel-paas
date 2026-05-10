import { useState, useEffect } from 'react'
import { Card, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Eye, EyeOff, Save, Loader2, ShieldAlert, AlertTriangle } from 'lucide-react'
import { cn } from '@/lib/utils'
import useTranslation from '@/lib/useTranslation'
import { projectsAPI } from '@/services/api'
import { toast } from 'sonner'

interface EnvironmentEditorProps {
  uid: string
  onSave?: () => void
}

export function EnvironmentEditor({ uid, onSave }: EnvironmentEditorProps) {
  const { t } = useTranslation()
  const [envContent, setEnvContent] = useState('')
  const [isEnvHidden, setIsEnvHidden] = useState(true)
  const [isSavingEnv, setIsSavingEnv] = useState(false)
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    fetchEnv()
  }, [uid])

  const fetchEnv = async () => {
    try {
      const response = await projectsAPI.getEnv(uid)
      setEnvContent(response.data.content)
    } catch (error) {
      toast.error(t('common.error'))
    } finally {
      setIsLoading(false)
    }
  }

  const handleSaveEnv = async () => {
    setIsSavingEnv(true)
    try {
      await projectsAPI.updateEnv(uid, envContent)
      toast.success(t('common.success'))
      if (onSave) onSave()
    } catch (error) {
      toast.error(t('common.error'))
    } finally {
      setIsSavingEnv(false)
    }
  }

  if (isLoading) {
    return (
      <Card className="flex items-center justify-center h-[600px] border-border/50 bg-card/50">
        <Loader2 className="w-8 h-8 animate-spin text-primary/50" />
      </Card>
    )
  }

  return (
    <Card className="flex flex-col h-[600px] overflow-hidden border-border/50 shadow-sm bg-card">
      <CardHeader className="pb-4 flex flex-row items-center justify-between border-b border-border bg-card">
        <div>
          <CardTitle className="text-lg flex items-center gap-2">
            <ShieldAlert className="w-5 h-5 text-primary" />
            {t('projectDetail.tabs.secrets')}
          </CardTitle>
          <CardDescription className="text-xs">{t('projectDetail.secrets.desc')}</CardDescription>
        </div>
        <div className="flex items-center gap-3">
          <Button 
            variant="outline" 
            size="sm" 
            onClick={() => setIsEnvHidden(!isEnvHidden)} 
            className="h-9"
          >
            {isEnvHidden ? <Eye className="w-3.5 h-3.5 mr-2" /> : <EyeOff className="w-3.5 h-3.5 mr-2" />}
            <span className="text-[10px] font-bold uppercase tracking-wider">
              {isEnvHidden ? t('projectDetail.actions.reveal') : t('projectDetail.actions.hide')}
            </span>
          </Button>
          <Button
            size="sm"
            onClick={handleSaveEnv}
            disabled={isSavingEnv || isEnvHidden}
            className="h-9 px-6 bg-primary hover:bg-primary/90"
          >
            {isSavingEnv ? <Loader2 className="w-3.5 h-3.5 mr-2 animate-spin" /> : <Save className="w-3.5 h-3.5 mr-2" />}
            <span className="text-[10px] font-bold uppercase tracking-wider">{t('common.save')}</span>
          </Button>
        </div>
      </CardHeader>
      
      <div className="flex-1 relative bg-zinc-950 overflow-hidden">
        <textarea
          value={envContent}
          onChange={e => setEnvContent(e.target.value)}
          readOnly={isEnvHidden}
          spellCheck={false}
          autoComplete="off"
          autoCorrect="off"
          autoCapitalize="off"
          data-gramm="false"
          className={cn(
            "w-full h-full p-8 font-mono text-[13px] leading-relaxed resize-none bg-transparent text-zinc-300 outline-none overflow-y-auto custom-scrollbar",
            "selection:bg-primary/30 selection:text-white",
            isEnvHidden ? "opacity-0 select-none pointer-events-none" : "opacity-100"
          )}
          placeholder={t('projectDetail.secrets.placeholder')}
        />
        
        {isEnvHidden && (
          <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
            <div className="px-6 py-3 bg-zinc-900/80 border border-white/10 backdrop-blur-md rounded-full shadow-2xl flex items-center gap-3">
              <ShieldAlert className="w-4 h-4 text-primary" />
              <span className="text-[10px] font-bold tracking-[0.2em] uppercase text-zinc-400">
                {t('projectDetail.secrets.locked')}
              </span>
            </div>
          </div>
        )}
      </div>

      <div className="p-4 bg-amber-500/5 text-amber-600 dark:text-amber-500/80 text-[9px] font-bold uppercase tracking-[0.15em] border-t border-border/50 flex items-center justify-center gap-3">
        <AlertTriangle size={14} className="animate-pulse" /> 
        {t('projectDetail.secrets.redeployNote')}
      </div>
    </Card>
  )
}
