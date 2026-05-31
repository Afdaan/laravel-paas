import { 
  PackageOpen, 
  Database as DbIcon, 
  Search, 
  ArrowRight,
  Loader2
} from 'lucide-react'
import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { projectsAPI } from '../../services/api'
import { Project } from '../../types'
import DatabaseStudio from './DatabaseStudio'
import useTranslation from '../../lib/useTranslation'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export default function Databases() {
  const { t } = useTranslation()
  const [projects, setProjects] = useState<Project[]>([])
  const [selectedProjectId, setSelectedProjectId] = useState<number | string | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [search, setSearch] = useState('')

  const fetchProjects = useCallback(async () => {
    try {
      const response = await projectsAPI.listOwn()
      const data = response.data.data || []
      setProjects(data)
      if (data.length > 0) {
        setSelectedProjectId(data[0].uid)
      }
    } catch (error) {
      toast.error(t('common.error'))
    } finally {
      setIsLoading(false)
    }
  }, [t])

  useEffect(() => {
    fetchProjects()
  }, [fetchProjects])

  const filteredProjects = projects.filter(p => 
    p.name.toLowerCase().includes(search.toLowerCase()) || 
    p.database_name?.toLowerCase().includes(search.toLowerCase())
  )

  const selectedProject = projects.find(p => p.uid === selectedProjectId || p.id === Number(selectedProjectId))

  if (isLoading) {
    return (
      <div className="min-h-[60vh] flex flex-col items-center justify-center gap-4">
        <Loader2 className="w-10 h-10 text-primary animate-spin" />
        <p className="text-muted-foreground font-bold uppercase tracking-widest text-[10px] animate-pulse">{t('databaseManager.loading')}</p>
      </div>
    )
  }

  if (projects.length === 0) {
    return (
      <div className="min-h-[60vh] flex flex-col items-center justify-center gap-6 animate-in fade-in duration-500">
        <div className="w-20 h-20 rounded-3xl bg-muted border flex items-center justify-center">
          <PackageOpen className="w-10 h-10 text-muted-foreground" />
        </div>
        <div className="text-center max-w-sm space-y-2">
          <h3 className="text-2xl font-bold tracking-tight">{t('databaseManager.noProjectsFound')}</h3>
          <p className="text-muted-foreground text-sm font-medium leading-relaxed italic">{t('databaseManager.noProjectsDesc')}</p>
        </div>
        <Link to="/projects/new" className={cn(buttonVariants({ variant: 'outline' }), "mt-4")}>
           {t('databaseManager.initFirst')}
        </Link>
      </div>
    )
  }

  return (
    <div className="h-[calc(100vh-140px)] flex flex-col lg:flex-row gap-8 animate-in fade-in duration-500">
      
      {/* Sidebar - Project Selection */}
      <div className="w-full lg:w-80 flex-shrink-0 flex flex-col gap-6">
         <Card className="flex flex-col overflow-hidden h-full">
            <CardHeader className="bg-muted/30 border-b pb-4">
               <div className="flex items-center gap-3">
                  <div className="w-10 h-10 rounded-xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary">
                    <DbIcon className="w-5 h-5" />
                  </div>
                  <div>
                    <h2 className="text-lg font-bold tracking-tight uppercase">{t('common.databases')}</h2>
                    <p className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">{t('databaseManager.activeInstances')}</p>
                  </div>
               </div>
               <div className="relative mt-6">
                 <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
                 <Input 
                   placeholder={t('databaseManager.searchSchema')} 
                   value={search}
                   onChange={(e) => setSearch(e.target.value)}
                   className="pl-9 h-10 text-xs font-bold uppercase tracking-widest"
                 />
               </div>
            </CardHeader>
            
            <CardContent className="flex-1 overflow-y-auto p-2 space-y-2 pt-4 scrollbar-thin">
               {filteredProjects.length > 0 ? (
                 filteredProjects.map(p => (
                   <button
                     key={p.uid}
                     onClick={() => setSelectedProjectId(p.uid)}
                     className={cn(
                        "w-full text-left p-4 rounded-xl transition-all border group focus:outline-none",
                        selectedProjectId === p.uid 
                           ? 'bg-primary/10 border-primary/30 shadow-sm' 
                           : 'border-transparent hover:bg-muted hover:border-border'
                     )}
                   >
                     <div className="flex items-center justify-between mb-2">
                       <span className={cn(
                          "font-bold text-xs uppercase tracking-tight",
                          selectedProjectId === p.uid ? 'text-primary' : 'text-foreground'
                       )}>
                         {p.name}
                       </span>
                       <div className={cn(
                          "w-1.5 h-1.5 rounded-full",
                          p.status === 'running' ? 'bg-emerald-500 animate-pulse' : 'bg-muted-foreground/30'
                       )} />
                     </div>
                     <div className="flex items-center gap-2 text-[10px] text-muted-foreground font-medium uppercase tracking-tight truncate">
                       <DbIcon className="w-3 h-3 text-primary/50 group-hover:text-primary" />
                       db_{p.database_name || '...'}
                     </div>
                   </button>
                 ))
               ) : (
                 <div className="text-center py-12 text-muted-foreground font-bold uppercase tracking-widest text-[10px] italic">
                   {t('databaseManager.noClusters')}
                 </div>
               )}
            </CardContent>
         </Card>
      </div>

      {/* Main Content - Selected Node Manager */}
      <Card className="flex-1 bg-muted/10 overflow-hidden flex flex-col relative border-muted/30">
        {selectedProject ? (
          <div className="flex-1 overflow-auto p-6 scrollbar-thin">
             <DatabaseStudio embedded={true} projectId={selectedProjectId} />
          </div>
        ) : (
          <div className="h-full flex flex-col items-center justify-center text-muted-foreground gap-6 opacity-40 animate-pulse">
             <div className="w-20 h-20 rounded-[2.5rem] bg-muted border flex items-center justify-center">
               <ArrowRight className="w-8 h-8 rotate-90 lg:rotate-0" />
             </div>
             <p className="text-[10px] font-bold uppercase tracking-[0.4em]">{t('databaseManager.selectTarget')}</p>
          </div>
        )}
      </Card>

    </div>
  )
}
