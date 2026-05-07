import React, { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { feedbackAPI } from '../../services/api'
import { AxiosError } from 'axios'
import {
  MessageSquare,
  Send,
  History as HistoryIcon,
  AlertTriangle,
  Lightbulb,
  User,
  Zap,
  Clock,
  Layout,
  Terminal,
  Bug,
  Loader2
} from 'lucide-react'
import useTranslation from '../../lib/useTranslation'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

interface FeedbackItem {
  id: number;
  title: string;
  content: string;
  type: 'suggestion' | 'bug' | 'trouble';
  status: 'pending' | 'in_progress' | 'resolved' | 'closed';
  created_at: string;
}

interface FeedbackForm {
  title: string;
  content: string;
  type: string;
}

const StudentFeedback = () => {
  const { t } = useTranslation()
  const [feedback, setFeedback] = useState<FeedbackItem[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const [formData, setFormData] = useState<FeedbackForm>({
    title: '',
    content: '',
    type: 'suggestion'
  })
  const [validationErrors, setValidationErrors] = useState<{ title?: string; content?: string }>({})

  const fetchFeedback = useCallback(async () => {
    setIsLoading(true)
    try {
      const res = await feedbackAPI.listOwn()
      setFeedback(res.data || [])
    } catch (error) {
      toast.error(t('common.error'))
    } finally {
      setIsLoading(false)
    }
  }, [t])

  useEffect(() => {
    fetchFeedback()
  }, [fetchFeedback])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    const errors: { title?: string; content?: string } = {}
    if (!formData.title.trim()) errors.title = t('feedback.subjectRequired')
    if (!formData.content.trim()) errors.content = t('feedback.detailsRequired')

    if (Object.keys(errors).length > 0) {
      setValidationErrors(errors)
      return
    }

    setIsSubmitting(true)
    try {
      await feedbackAPI.submit(formData)
      toast.success(t('feedback.success'))
      setFormData({ title: '', content: '', type: 'suggestion' })
      setValidationErrors({})
      fetchFeedback()
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: string }>
      toast.error(axiosError.response?.data?.error || t('common.error'))
    } finally {
      setIsSubmitting(false)
    }
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'resolved':
        return <Badge className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20 uppercase text-[9px] tracking-widest font-bold">{t('feedback.status.resolved')}</Badge>
      case 'in_progress':
        return <Badge className="bg-amber-500/10 text-amber-500 border-amber-500/20 uppercase text-[9px] tracking-widest font-bold">{t('feedback.status.inPreview')}</Badge>
      default:
        return <Badge variant="outline" className="text-muted-foreground uppercase text-[9px] tracking-widest font-bold">{t('feedback.status.pending')}</Badge>
    }
  }

  const getTypeIcon = (type: string) => {
    switch (type) {
      case 'bug': return <Bug className="w-3.5 h-3.5 text-rose-500" />;
      case 'trouble': return <AlertTriangle className="w-3.5 h-3.5 text-amber-500" />;
      default: return <Lightbulb className="w-3.5 h-3.5 text-primary" />;
    }
  }

  return (
    <div className="space-y-12 animate-in fade-in duration-500 max-w-7xl mx-auto pb-20">
      <div className="space-y-2">
        <h1 className="text-4xl font-bold tracking-tight">{t('feedback.title')}</h1>
        <p className="text-muted-foreground text-lg font-medium">{t('feedback.subtitle')}</p>
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-12 gap-10">
        <div className="xl:col-span-7 space-y-10">
          <Card className="p-8">
            <form onSubmit={handleSubmit} className="space-y-8">
              <div className="space-y-3">
                <Label htmlFor="title" className="text-[10px] font-bold text-muted-foreground uppercase tracking-[0.2em] flex items-center gap-2">
                  <MessageSquare className="w-3 h-3 text-primary" />
                  {t('feedback.subject')}
                </Label>
                <Input
                  id="title"
                  placeholder={t('feedback.subjectPlaceholder')}
                  value={formData.title}
                  onChange={e => setFormData({ ...formData, title: e.target.value })}
                  className={cn(validationErrors.title && "border-destructive focus-visible:ring-destructive")}
                />
                {validationErrors.title && (
                  <p className="text-xs text-destructive font-medium pl-1">{validationErrors.title}</p>
                )}
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                <div className="space-y-3">
                  <Label className="text-[10px] font-bold text-muted-foreground uppercase tracking-[0.2em] flex items-center gap-2">
                    <Layout className="w-3 h-3 text-primary" />
                    {t('feedback.category')}
                  </Label>
                  <Select
                    value={formData.type}
                    onValueChange={(val) => setFormData({ ...formData, type: val || 'suggestion' })}
                  >
                    <SelectTrigger className="h-12 w-full border-muted-foreground/20">
                      <SelectValue placeholder={t('feedback.categoryPlaceholder')}>
                        {formData.type === 'suggestion' && t('feedback.suggestion')}
                        {formData.type === 'bug' && t('feedback.bug')}
                        {formData.type === 'trouble' && t('feedback.issue')}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="suggestion">{t('feedback.suggestion')}</SelectItem>
                      <SelectItem value="bug">{t('feedback.bug')}</SelectItem>
                      <SelectItem value="trouble">{t('feedback.issue')}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="flex items-center">
                  <div className="p-5 rounded-xl bg-primary/5 border border-primary/10 flex items-center gap-4 w-full">
                    <Zap className="w-5 h-5 text-primary" />
                    <p className="text-[10px] text-muted-foreground font-medium leading-relaxed italic" dangerouslySetInnerHTML={{ __html: t('feedback.helpText').replace('Platform', '<span class="text-primary font-bold">Platform</span>') }} />
                  </div>
                </div>
              </div>

              <div className="space-y-3">
                <Label htmlFor="content" className="text-[10px] font-bold text-muted-foreground uppercase tracking-[0.2em] flex items-center gap-2">
                  <Terminal className="w-3 h-3 text-primary" />
                  {t('feedback.details')}
                </Label>
                <Textarea
                  id="content"
                  rows={6}
                  placeholder={t('feedback.detailsPlaceholder')}
                  value={formData.content}
                  onChange={e => setFormData({ ...formData, content: e.target.value })}
                  className={cn("resize-none font-medium", validationErrors.content && "border-destructive focus-visible:ring-destructive")}
                />
                {validationErrors.content && (
                  <p className="text-xs text-destructive font-medium pl-1">{validationErrors.content}</p>
                )}
              </div>

              <Button
                type="submit"
                disabled={isSubmitting}
                className="w-full h-14 font-bold uppercase tracking-[0.2em] gap-3"
              >
                {isSubmitting ? <Loader2 className="w-5 h-5 animate-spin" /> : <Send className="w-4 h-4" />}
                {isSubmitting ? t('feedback.submitting') : t('feedback.dispatch')}
              </Button>
            </form>
          </Card>
        </div>

        <div className="xl:col-span-5 space-y-6">
          <div className="flex items-center justify-between px-2">
            <div className="flex items-center gap-2">
              <HistoryIcon className="w-4 h-4 text-muted-foreground" />
              <h3 className="text-xs font-bold text-muted-foreground uppercase tracking-[0.2em]">{t('feedback.history')}</h3>
            </div>
            <Badge variant="secondary" className="text-[9px] font-bold uppercase tracking-widest">{t('feedback.logged', { count: feedback.length })}</Badge>
          </div>

          <div className="space-y-4 max-h-[800px] overflow-y-auto pr-2 custom-scrollbar">
            {isLoading ? (
              <div className="flex flex-col items-center justify-center p-20 gap-4 opacity-50">
                <Loader2 className="w-8 h-8 animate-spin text-primary" />
                <p className="text-[10px] font-bold uppercase tracking-widest">{t('feedback.loading')}</p>
              </div>
            ) : feedback.length === 0 ? (
              <Card className="p-12 text-center border-dashed flex flex-col items-center gap-4 bg-muted/20">
                <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center">
                  <MessageSquare className="w-8 h-8 text-muted-foreground" />
                </div>
                <p className="text-muted-foreground text-xs font-bold uppercase tracking-widest">{t('feedback.noRecords')}</p>
              </Card>
            ) : (
              feedback.map((item: FeedbackItem) => (
                <Card key={item.id} className="p-6 transition-all duration-300 hover:border-primary/30 relative overflow-hidden group">
                  <div className="flex items-center justify-between mb-4">
                    <Badge variant="outline" className="gap-1.5 py-1 px-2 uppercase text-[9px] font-bold border-primary/20 bg-primary/5">
                      {getTypeIcon(item.type)}
                      {t(`feedback.${item.type}` as "feedback.suggestion" | "feedback.bug" | "feedback.trouble")}
                    </Badge>
                    {getStatusBadge(item.status)}
                  </div>

                  <h4 className="text-sm font-bold truncate mb-2 uppercase tracking-tight group-hover:text-primary transition-colors">{item.title}</h4>
                  <p className="text-[11px] text-muted-foreground font-medium leading-relaxed italic line-clamp-2 mb-4">"{item.content}"</p>

                  <div className="flex items-center justify-between pt-4 border-t border-muted">
                    <div className="flex items-center gap-2 text-muted-foreground">
                      <Clock className="w-3.5 h-3.5 text-primary/50" />
                      <span className="text-[9px] font-bold uppercase tracking-widest">{new Date(item.created_at).toLocaleDateString()}</span>
                    </div>
                    <div className="w-6 h-6 rounded-full bg-muted flex items-center justify-center border">
                      <User className="w-3 h-3 text-muted-foreground" />
                    </div>
                  </div>
                </Card>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

export default StudentFeedback
