import { useState, useEffect, memo, useCallback } from 'react'
import { feedbackAPI } from '../../services/api'
import { toast } from 'sonner'
import {
  MessageSquare,
  RotateCw,
  Trash2,
  User,
  Clock,
  ShieldAlert,
  Sparkles,
  Bug,
  AlertTriangle,
  Lightbulb,
  Search,
  Loader2
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardFooter } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import useTranslation from '../../lib/useTranslation'
import ConfirmationModal from '../../components/ConfirmationModal'

interface FeedbackUser {
  name: string;
  email: string;
}

interface FeedbackData {
  id: number;
  type: string;
  title: string;
  content: string;
  status: string;
  created_at: string;
  user?: FeedbackUser;
}

const AdminFeedback = () => {
  const { t } = useTranslation()
  const [feedback, setFeedback] = useState<FeedbackData[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [filterStatus, setFilterStatus] = useState<string>('all')
  const [filterType, setFilterType] = useState<string>('all')
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false)
  const [targetDeleteId, setTargetDeleteId] = useState<number | null>(null)

  const fetchFeedback = useCallback(async () => {
    setIsLoading(true)
    try {
      const params: Record<string, string> = {}
      if (filterStatus !== 'all') params.status = filterStatus
      if (filterType !== 'all') params.type = filterType

      const res = await feedbackAPI.listAll(params)
      setFeedback(res.data || [])
    } catch (error) {
      console.error('Failed to fetch feedback:', error)
      toast.error(t('admin.feedback.loadError'))
    } finally {
      setIsLoading(false)
    }
  }, [filterStatus, filterType, t])

  useEffect(() => {
    fetchFeedback()
  }, [fetchFeedback])

  const handleUpdateStatus = async (id: number, status: string) => {
    try {
      await feedbackAPI.updateStatus(id, status)
      toast.success(t('admin.feedback.updateSuccess'))
      fetchFeedback()
    } catch (error) {
      toast.error(t('admin.feedback.updateError'))
    }
  }

  const handleDeleteTrigger = (id: number) => {
    setTargetDeleteId(id)
    setIsDeleteModalOpen(true)
  }

  const confirmDelete = async () => {
    if (!targetDeleteId) return
    try {
      await feedbackAPI.delete(targetDeleteId)
      toast.success(t('admin.feedback.purgeSuccess'))
      fetchFeedback()
    } catch (error) {
      toast.error(t('admin.feedback.purgeError'))
    } finally {
      setIsDeleteModalOpen(false)
      setTargetDeleteId(null)
    }
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6 pb-4 border-b">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('admin.feedback.title')}</h1>
          <p className="text-muted-foreground">{t('admin.feedback.desc')}</p>
        </div>

        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2 bg-muted/30 border p-2 rounded-xl text-xs font-bold uppercase tracking-widest text-muted-foreground">
            <div className="flex items-center gap-2 px-3 border-r">
              <MessageSquare className="w-3.5 h-3.5 text-indigo-500" />
              {t('admin.feedback.submissions', { count: feedback?.length || 0 })}
            </div>
            <div className="flex items-center gap-2 px-3">
              <ShieldAlert className="w-3.5 h-3.5 text-rose-500" />
              {t('admin.feedback.critical', { count: (feedback || []).filter(f => f.type === 'bug').length })}
            </div>
          </div>

          <Button variant="outline" size="icon" onClick={fetchFeedback}>
            <RotateCw className="w-4 h-4 text-muted-foreground" />
          </Button>
        </div>
      </div>

      <Card>
        <div className="p-4 flex flex-col md:flex-row items-center justify-between gap-4">
          <div className="relative flex-1 w-full max-w-xl">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t('admin.feedback.filterPlaceholder')}
              className="pl-9 w-full"
            />
          </div>

          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-muted-foreground">{t('admin.feedback.status')}:</span>
              <Select value={filterStatus} onValueChange={(v) => setFilterStatus(v || 'all')}>
                <SelectTrigger className="w-[140px]">
                  <SelectValue placeholder={t('admin.feedback.allStates')} />
                </SelectTrigger>
                <SelectContent align="start" alignItemWithTrigger={false} className="min-w-[var(--radix-select-trigger-width)] bg-popover/98 backdrop-blur-lg border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72">
                  <SelectItem value="all">{t('admin.feedback.allStates')}</SelectItem>
                  <SelectItem value="pending">{t('feedback.status.pending')}</SelectItem>
                  <SelectItem value="in_review">{t('feedback.status.inPreview')}</SelectItem>
                  <SelectItem value="resolved">{t('feedback.status.resolved')}</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="flex items-center gap-2">
              <span className="text-sm font-medium text-muted-foreground">{t('admin.feedback.category')}:</span>
              <Select value={filterType} onValueChange={(v) => setFilterType(v || 'all')}>
                <SelectTrigger className="w-[160px]">
                  <SelectValue placeholder={t('admin.feedback.allCategories')} />
                </SelectTrigger>
                <SelectContent align="start" alignItemWithTrigger={false} className="min-w-[var(--radix-select-trigger-width)] bg-popover/98 backdrop-blur-lg border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72">
                  <SelectItem value="all">
                    <div className="flex items-center gap-2"><Sparkles className="w-4 h-4" /> {t('admin.feedback.allCategories')}</div>
                  </SelectItem>
                  <SelectItem value="suggestion">
                    <div className="flex items-center gap-2"><Lightbulb className="w-4 h-4 text-indigo-500" /> {t('admin.feedback.suggestion')}</div>
                  </SelectItem>
                  <SelectItem value="bug">
                    <div className="flex items-center gap-2"><Bug className="w-4 h-4 text-rose-500" /> {t('admin.feedback.criticalBug')}</div>
                  </SelectItem>
                  <SelectItem value="trouble">
                    <div className="flex items-center gap-2"><AlertTriangle className="w-4 h-4 text-amber-500" /> {t('admin.feedback.infraIssue')}</div>
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        </div>
      </Card>

      {isLoading ? (
        <div className="flex flex-col items-center justify-center h-80 gap-6">
          <Loader2 className="w-12 h-12 text-primary animate-spin" />
          <p className="text-muted-foreground text-xs font-bold uppercase tracking-widest animate-pulse">{t('admin.feedback.syncing')}</p>
        </div>
      ) : (!feedback || feedback.length === 0) ? (
        <Card className="py-24 flex flex-col items-center justify-center text-center">
          <div className="w-20 h-20 bg-muted/50 rounded-full flex items-center justify-center mb-6">
            <Sparkles className="w-10 h-10 text-muted-foreground opacity-50" />
          </div>
          <h3 className="font-semibold text-lg">{t('admin.feedback.noRecords')}</h3>
          <p className="text-muted-foreground mt-2 max-w-sm">{t('admin.feedback.noRecordsDesc')}</p>
        </Card>
      ) : (
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
          {feedback.map(item => (
            <FeedbackCard
              key={item.id}
              item={item}
              onUpdate={handleUpdateStatus}
              onDelete={handleDeleteTrigger}
            />
          ))}
        </div>
      )}

      <ConfirmationModal
        isOpen={isDeleteModalOpen}
        onClose={() => {
          setIsDeleteModalOpen(false)
          setTargetDeleteId(null)
        }}
        onConfirm={confirmDelete}
        title={t('admin.feedback.deleteTitle')}
        message={t('admin.feedback.confirmDelete')}
        confirmText={t('admin.initCleanup')}
        type="danger"
      />
    </div>
  )
}

const FeedbackCard = memo(({ item, onUpdate, onDelete }: { item: FeedbackData, onUpdate: (id: number, status: string) => void, onDelete: (id: number) => void }) => {
  const { t } = useTranslation()
  
  const typeConfigs: Record<string, { color: string, icon: React.ElementType, glow: string, label: string }> = {
    bug: { color: 'text-rose-600', icon: Bug, glow: 'bg-rose-500/10', label: t('admin.feedback.criticalBug') },
    trouble: { color: 'text-amber-600', icon: AlertTriangle, glow: 'bg-amber-500/10', label: t('admin.feedback.infraIssue') },
    suggestion: { color: 'text-indigo-600', icon: Lightbulb, glow: 'bg-indigo-500/10', label: t('admin.feedback.suggestion') },
  }

  const config = typeConfigs[item.type] || typeConfigs.suggestion
  const Icon = config.icon

  return (
    <Card className="relative overflow-hidden group hover:border-border/80 transition-colors">
      <div className={`absolute top-0 right-0 w-32 h-32 blur-[80px] pointer-events-none transition-opacity duration-700 opacity-10 group-hover:opacity-30 ${config.glow}`} />

      <CardContent className="pt-6 relative z-10">
        <div className="flex items-start justify-between mb-6">
          <div className="flex items-center gap-4">
            <div className="w-10 h-10 bg-muted border rounded-lg flex items-center justify-center font-bold text-foreground">
              {item.user?.name?.charAt(0).toUpperCase() || 'U'}
            </div>
            <div>
              <p className="text-sm font-bold text-foreground">{item.user?.name || 'Unknown User'}</p>
              <div className="flex items-center gap-1.5 mt-0.5 text-muted-foreground">
                <User className="w-3 h-3" />
                <p className="text-xs">{item.user?.email || 'N/A'}</p>
              </div>
            </div>
          </div>

          <div className="flex flex-col items-end gap-2">
            <Badge variant={
              item.status === 'resolved' ? 'outline' :
                item.status === 'in_review' ? 'secondary' : 'default'
            } className={
              item.status === 'resolved' ? 'text-emerald-600 border-emerald-500/40 bg-emerald-500/10' :
                item.status === 'in_review' ? 'text-indigo-600 bg-indigo-500/10 hover:bg-indigo-500/20' : ''
            }>
              {item.status.replace('_', ' ').toUpperCase()}
            </Badge>
            <div className="flex items-center gap-1.5 text-muted-foreground mt-1">
              <Clock className="w-3 h-3" />
              <span className="text-[10px] font-bold uppercase tracking-wider">{new Date(item.created_at).toLocaleDateString()}</span>
            </div>
          </div>
        </div>

        <div className="space-y-4 mb-4">
          <div className="flex items-center gap-2">
            <div className={`w-6 h-6 rounded-md bg-background border flex items-center justify-center ${config.color} shadow-sm`}>
              <Icon className="w-3.5 h-3.5" />
            </div>
            <h3 className="font-semibold text-foreground tracking-tight line-clamp-1">{item.title}</h3>
          </div>

          <div className="bg-muted/50 p-4 rounded-lg border border-border/50 text-sm text-foreground/80 leading-relaxed italic">
            "{item.content}"
          </div>
        </div>
      </CardContent>

      <CardFooter className="border-t bg-muted/10 justify-between py-4">
        <div className="flex items-center gap-2">
          {item.status !== 'in_review' && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => onUpdate(item.id, 'in_review')}
              className="text-indigo-600"
            >
              {t('feedback.status.inPreview')}
            </Button>
          )}
          {item.status !== 'resolved' && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => onUpdate(item.id, 'resolved')}
              className="text-emerald-600 border-emerald-200 hover:bg-emerald-50"
            >
              {t('feedback.status.resolved')}
            </Button>
          )}
        </div>

        <Button
          variant="ghost"
          size="icon"
          onClick={() => onDelete(item.id)}
          className="text-destructive hover:bg-destructive/10 cursor-pointer"
        >
          <Trash2 className="w-4 h-4" />
        </Button>
      </CardFooter>
    </Card>
  )
})

export default AdminFeedback
