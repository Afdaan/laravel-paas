import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { databaseAPI } from '../../services/api'
import useTranslation from '../../lib/useTranslation'
import {
  Database,
  Search,
  ExternalLink,
  User,
  Layers,
  HardDrive,
  RefreshCw,
  Info,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Card, CardContent, CardFooter, CardHeader } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

interface AdminDatabaseInfo {
  project_id: number;
  project_name: string;
  student_name: string;
  database_name: string;
  table_count: number;
  size: string;
  status: string;
}

const StatusBadge = ({ status, t }: { status: string, t: any }) => {
  switch (status) {
    case 'running':
      return <Badge variant="outline" className="text-emerald-600 border-emerald-500/40 bg-emerald-500/10"><div className="w-1.5 h-1.5 rounded-full bg-emerald-500 mr-1.5 animate-pulse" /> {t('status.running')}</Badge>
    case 'building':
      return <Badge variant="outline" className="text-indigo-600 border-indigo-500/40 bg-indigo-500/10"><div className="w-1.5 h-1.5 rounded-full bg-indigo-500 mr-1.5 animate-pulse" /> {t('status.building')}</Badge>
    case 'pending':
      return <Badge variant="outline" className="text-amber-600 border-amber-500/40 bg-amber-500/10"><div className="w-1.5 h-1.5 rounded-full bg-amber-500 mr-1.5" /> {t('status.pending')}</Badge>
    case 'failed':
      return <Badge variant="destructive" className="bg-destructive/10 text-destructive hover:bg-destructive/20"><div className="w-1.5 h-1.5 rounded-full bg-destructive mr-1.5" /> {t('status.failed')}</Badge>
    case 'stopped':
    default:
      return <Badge variant="secondary"><div className="w-1.5 h-1.5 rounded-full bg-muted-foreground mr-1.5" /> {t('status.stopped')}</Badge>
  }
}

const AdminDatabases = () => {
  const { t } = useTranslation()
  const [databases, setDatabases] = useState<AdminDatabaseInfo[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [search, setSearch] = useState('')
  
  // Pagination
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(10)

  const fetchDatabases = useCallback(async () => {
    setIsLoading(true)
    try {
      const response = await databaseAPI.adminListAll()
      setDatabases(response.data.databases || [])
    } catch (error) {
      toast.error(t('common.loadError'))
    } finally {
      setIsLoading(false)
    }
  }, [t])

  useEffect(() => {
    fetchDatabases()
  }, [fetchDatabases])

  const filteredDatabases = databases.filter(db => 
    db.project_name.toLowerCase().includes(search.toLowerCase()) ||
    db.database_name.toLowerCase().includes(search.toLowerCase()) ||
    db.student_name.toLowerCase().includes(search.toLowerCase())
  )

  const total = filteredDatabases.length
  const totalPages = Math.ceil(total / limit)
  const paginatedDatabases = filteredDatabases.slice((page - 1) * limit, page * limit)

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('admin.databases.title')}</h1>
          <p className="text-muted-foreground">{t('admin.databases.desc')}</p>
        </div>

        <div className="flex items-center gap-4 bg-muted/30 border p-2 rounded-xl">
           <div className="flex items-center gap-2 px-4 border-r">
             <Database className="w-4 h-4 text-primary" />
             <span className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
               {databases.length} {t('admin.databases.provisioned')}
             </span>
           </div>
           <div className="flex items-center gap-2 px-4">
             <Layers className="w-4 h-4 text-emerald-500" />
             <span className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
               {databases.reduce((acc, db) => acc + db.table_count, 0)} {t('admin.databases.tables')}
             </span>
           </div>
        </div>
      </div>

      <Card className="gap-0 py-0 border border-border/50 shadow-sm bg-card/50 backdrop-blur-xl overflow-hidden">
        <CardHeader className="border-b border-border/50 py-8 px-8 bg-muted/20">
          <div className="flex flex-col md:flex-row items-center justify-between gap-6">
            <div className="relative flex-1 w-full max-w-2xl">
              <Search className="w-5 h-5 absolute left-4 top-1/2 -translate-y-1/2 text-muted-foreground/50" />
              <Input
                placeholder={t('common.search')}
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value)
                  setPage(1)
                }}
                className="pl-12 h-12 bg-background/50 border-border rounded-xl focus-visible:ring-primary/20"
              />
            </div>
 
            <Button variant="outline" size="icon" onClick={fetchDatabases} disabled={isLoading} className="h-12 w-12 rounded-xl border-border/50 bg-background/50">
              <RefreshCw className={`w-5 h-5 ${isLoading ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        </CardHeader>
 
        <CardContent className="p-0">
          <div className="overflow-x-auto">
          <Table className="min-w-[1080px]">
            <TableHeader>
              <TableRow className="bg-muted/40 border-b border-border/50 hover:bg-muted/40 transition-none">
                <TableHead className="h-14 px-8 text-[10px] font-bold uppercase tracking-[0.2em] text-muted-foreground/80">{t('admin.databases.identity')}</TableHead>
                <TableHead className="h-14 px-8 text-[10px] font-bold uppercase tracking-[0.2em] text-muted-foreground/80">{t('common.student')}</TableHead>
                <TableHead className="h-14 px-8 text-[10px] font-bold uppercase tracking-[0.2em] text-muted-foreground/80">{t('admin.databases.connection')}</TableHead>
                <TableHead className="h-14 px-8 text-center text-[10px] font-bold uppercase tracking-[0.2em] text-muted-foreground/80">{t('admin.databases.tables')}</TableHead>
                <TableHead className="h-14 px-8 text-center text-[10px] font-bold uppercase tracking-[0.2em] text-muted-foreground/80">{t('common.size')}</TableHead>
                <TableHead className="h-14 px-8 text-right text-[10px] font-bold uppercase tracking-[0.2em] text-muted-foreground/80">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow className="border-border/50">
                  <TableCell colSpan={6} className="h-64 text-center">
                    <div className="flex flex-col items-center justify-center gap-4 opacity-30">
                      <RefreshCw className="w-10 h-10 animate-spin" />
                      <span className="text-[10px] font-bold uppercase tracking-[0.3em]">{t('common.loading')}</span>
                    </div>
                  </TableCell>
                </TableRow>
              ) : paginatedDatabases.length === 0 ? (
                <TableRow className="border-border/50">
                  <TableCell colSpan={6} className="h-64 text-center">
                    <div className="flex flex-col items-center justify-center text-muted-foreground opacity-30">
                      <div className="w-20 h-20 bg-muted/50 rounded-full flex items-center justify-center mb-6">
                        <Database className="w-10 h-10" />
                      </div>
                      <span className="font-bold uppercase tracking-[0.3em] text-xs">{t('common.noData')}</span>
                    </div>
                  </TableCell>
                </TableRow>
              ) : paginatedDatabases.map((db) => (
                <TableRow key={db.project_id} className="group hover:bg-muted/20 border-border/50 transition-colors">
                  <TableCell className="px-8 py-5">
                    <div className="flex items-center gap-5">
                      <div className={`w-12 h-12 rounded-2xl border flex items-center justify-center transition-all group-hover:scale-110 duration-500 shadow-lg ${db.status === 'running' ? 'bg-emerald-500/10 text-emerald-500 border-emerald-500/20' : 'bg-muted/30 text-muted-foreground/60 border-border/50'
                        }`}>
                        <Database className="w-6 h-6" />
                      </div>
                      <div className="flex flex-col justify-center min-w-0">
                        <span className="text-base font-bold truncate max-w-[220px] group-hover:text-primary transition-colors">{db.database_name}</span>
                        <span className="text-[10px] text-muted-foreground/60 uppercase font-black tracking-[0.15em] mt-1">
                          {t('admin.databases.mysqlInstance')}
                        </span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="px-8 py-5">
                    <div className="flex items-center gap-4">
                      <div className="w-10 h-10 rounded-full bg-muted/50 border border-border/50 flex items-center justify-center text-muted-foreground">
                        <User className="w-5 h-5" />
                      </div>
                      <div className="flex flex-col justify-center min-w-0">
                        <span className="text-xs font-black uppercase tracking-tight truncate max-w-[160px] text-foreground/90">{db.student_name}</span>
                        <span className="text-[10px] text-muted-foreground/50 uppercase font-bold mt-0.5">{t('common.student')}</span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="px-8 py-5">
                    <div className="flex flex-col items-start gap-2">
                       <span className="text-sm font-bold text-foreground/80 truncate max-w-[220px]">{db.project_name}</span>
                       <StatusBadge status={db.status} t={t} />
                    </div>
                  </TableCell>
                  <TableCell className="px-8 py-5 text-center whitespace-nowrap">
                    <div className="inline-flex items-center gap-2 text-xs font-black rounded-xl border border-border/50 bg-muted/20 px-3 py-1.5 shadow-inner">
                       <Layers className="w-4 h-4 text-muted-foreground/60" />
                       {db.table_count}
                     </div>
                  </TableCell>
                  <TableCell className="px-8 py-5 text-center whitespace-nowrap">
                    <div className="inline-flex items-center gap-2 text-xs font-black rounded-xl border border-border/50 bg-muted/20 px-3 py-1.5 shadow-inner">
                       <HardDrive className="w-4 h-4 text-muted-foreground/60" />
                       {db.size}
                     </div>
                  </TableCell>
                  <TableCell className="px-8 py-5 text-right">
                    <div className="flex items-center justify-end gap-3">
                      <Link to={`/projects/${db.project_id}/database`}>
                        <Button variant="outline" size="sm" className="h-9 px-4 rounded-xl font-bold bg-primary/5 border-primary/20 text-primary hover:bg-primary hover:text-white transition-all">
                           {t('admin.databases.manage')}
                        </Button>
                      </Link>
                      <Link to={`/projects/${db.project_id}`}>
                        <Button variant="ghost" size="icon" className="h-9 w-9 rounded-xl text-muted-foreground/40 hover:text-primary hover:bg-primary/5 transition-all">
                           <ExternalLink className="w-4 h-4" />
                        </Button>
                      </Link>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
        </CardContent>

        {totalPages > 0 && (
          <CardFooter className="p-4 border-t flex flex-col sm:flex-row justify-between items-center gap-4 bg-muted/10">
            <div className="flex items-center gap-6">
              <div className="flex items-center gap-2 text-sm text-muted-foreground font-medium">
                <Info className="w-4 h-4 text-primary" />
                {t('admin.databases.summary', { 
                  start: (page - 1) * limit + 1,
                  end: Math.min(page * limit, total),
                  total: total 
                })}
              </div>
              <div className="flex items-center space-x-2">
                <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{t('common.rowsPerPage') || 'Rows per page'}</p>
                <Select
                  value={limit.toString()}
                  onValueChange={(value) => {
                    setLimit(Number(value))
                    setPage(1)
                  }}
                >
                  <SelectTrigger size="sm" className="h-8 w-[82px] justify-between">
                    <SelectValue placeholder={limit} />
                  </SelectTrigger>
                  <SelectContent
                    side="top"
                    align="end"
                    className="min-w-[120px] max-h-[220px] rounded-lg p-1 shadow-lg"
                  >
                    {[10, 15, 20, 25, 30, 40, 50, 75, 100].map((pageSize) => (
                      <SelectItem
                        key={pageSize}
                        value={`${pageSize}`}
                        className="rounded-md py-1.5 px-2 text-sm"
                      >
                        {pageSize}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="flex items-center space-x-6 lg:space-x-8">
              <div className="flex w-[100px] items-center justify-center text-sm font-medium">
                Page {page} of {Math.max(1, totalPages)}
              </div>
              <div className="flex items-center space-x-2">
                <Button
                  variant="outline"
                  className="hidden h-8 w-8 p-0 lg:flex"
                  onClick={() => setPage(1)}
                  disabled={page === 1}
                >
                  <span className="sr-only">Go to first page</span>
                  <ChevronsLeft className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  className="h-8 w-8 p-0"
                  onClick={() => setPage(p => Math.max(1, p - 1))}
                  disabled={page === 1}
                >
                  <span className="sr-only">Go to previous page</span>
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  className="h-8 w-8 p-0"
                  onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                  disabled={page === totalPages || totalPages === 0}
                >
                  <span className="sr-only">Go to next page</span>
                  <ChevronRight className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  className="hidden h-8 w-8 p-0 lg:flex"
                  onClick={() => setPage(totalPages)}
                  disabled={page === totalPages || totalPages === 0}
                >
                  <span className="sr-only">Go to last page</span>
                  <ChevronsRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </CardFooter>
        )}
      </Card>
    </div>
  )
}

export default AdminDatabases
