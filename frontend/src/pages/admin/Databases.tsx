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
import { Card } from '@/components/ui/card'
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

      <Card>
        <div className="p-6 border-b flex flex-col md:flex-row items-center justify-between gap-4">
          <div className="relative flex-1 w-full max-w-2xl">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t('common.search')}
              value={search}
              onChange={(e) => {
                setSearch(e.target.value)
                setPage(1)
              }}
              className="pl-9"
            />
          </div>

          <Button variant="outline" size="icon" onClick={fetchDatabases} disabled={isLoading}>
            <RefreshCw className={`w-4 h-4 ${isLoading ? 'animate-spin' : ''}`} />
          </Button>
        </div>

        <div className="overflow-x-auto min-h-[400px]">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('admin.databases.identity')}</TableHead>
                <TableHead>{t('common.student')}</TableHead>
                <TableHead>{t('admin.databases.connection')}</TableHead>
                <TableHead>{t('admin.databases.tables')}</TableHead>
                <TableHead>{t('common.size')}</TableHead>
                <TableHead className="text-right">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-32 text-center text-muted-foreground font-medium uppercase tracking-widest text-xs">
                    {t('common.loading')}
                  </TableCell>
                </TableRow>
              ) : paginatedDatabases.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-64 text-center">
                    <div className="flex flex-col items-center justify-center text-muted-foreground">
                      <div className="w-16 h-16 bg-muted/50 rounded-full flex items-center justify-center mb-4">
                        <Database className="w-8 h-8 opacity-50" />
                      </div>
                      <span className="font-semibold text-sm">{t('common.noData')}</span>
                    </div>
                  </TableCell>
                </TableRow>
              ) : paginatedDatabases.map((db) => (
                <TableRow key={db.project_id}>
                  <TableCell>
                    <div className="flex items-center gap-4">
                      <div className={`w-10 h-10 rounded-lg border flex items-center justify-center ${db.status === 'running' ? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20' : 'bg-muted/30 text-muted-foreground border-border'
                        }`}>
                        <Database className="w-5 h-5" />
                      </div>
                      <div className="flex flex-col justify-center">
                        <span className="text-sm font-semibold truncate max-w-[200px]">{db.database_name}</span>
                        <span className="text-[10px] text-muted-foreground uppercase font-bold tracking-widest mt-0.5">
                          {t('admin.databases.mysqlInstance')}
                        </span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-full bg-muted/50 flex items-center justify-center text-muted-foreground">
                        <User className="w-4 h-4" />
                      </div>
                      <div className="flex flex-col justify-center">
                        <span className="text-xs font-bold uppercase tracking-tight">{db.student_name}</span>
                        <span className="text-[10px] text-muted-foreground uppercase mt-0.5">{t('common.student')}</span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col items-start gap-1.5">
                       <span className="text-sm font-medium">{db.project_name}</span>
                       <StatusBadge status={db.status} t={t} />
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1.5 text-sm font-medium">
                       <Layers className="w-4 h-4 text-muted-foreground" />
                       {db.table_count}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1.5 text-sm font-medium">
                       <HardDrive className="w-4 h-4 text-muted-foreground" />
                       {db.size}
                    </div>
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-2">
                      <Link to={`/projects/${db.project_id}/database`}>
                        <Button variant="outline" size="sm">
                           {t('admin.databases.manage')}
                        </Button>
                      </Link>
                      <Link to={`/projects/${db.project_id}`}>
                        <Button variant="ghost" size="icon" className="h-8 w-8 text-muted-foreground hover:text-primary">
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

        {totalPages > 1 && (
          <div className="p-4 border-t flex flex-col sm:flex-row justify-between items-center gap-4 bg-muted/10">
            <div className="flex items-center gap-2 text-sm text-muted-foreground font-medium">
              <Info className="w-4 h-4 text-primary" />
              {t('admin.databases.summary', { 
                start: (page - 1) * limit + 1,
                end: Math.min(page * limit, total),
                total: total 
              })}
            </div>

            <div className="flex-1" />

            <div className="flex items-center gap-6">
              <div className="flex items-center space-x-2">
                <p className="text-xs font-bold uppercase tracking-widest text-muted-foreground mr-1">
                  {t('common.rowsPerPage')}
                </p>
                <Select
                  value={limit.toString()}
                  onValueChange={(value) => {
                    setLimit(Number(value))
                    setPage(1)
                  }}
                >
                  <SelectTrigger className="h-8 w-[70px] bg-background">
                    <SelectValue placeholder={limit} />
                  </SelectTrigger>
                  <SelectContent side="top">
                    {[10, 20, 30, 50, 100].map((pageSize) => (
                      <SelectItem key={pageSize} value={`${pageSize}`}>
                        {pageSize}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="flex items-center space-x-2">
              <Button
                variant="outline"
                className="h-8 w-8 p-0"
                onClick={() => setPage(1)}
                disabled={page === 1}
              >
                <ChevronsLeft className="h-4 w-4" />
              </Button>
              <Button
                variant="outline"
                className="h-8 w-8 p-0"
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page === 1}
              >
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <div className="text-sm font-medium px-2">
                {page} / {totalPages}
              </div>
              <Button
                variant="outline"
                className="h-8 w-8 p-0"
                onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                disabled={page === totalPages}
              >
                <ChevronRight className="h-4 w-4" />
              </Button>
              <Button
                variant="outline"
                className="h-8 w-8 p-0"
                onClick={() => setPage(totalPages)}
                disabled={page === totalPages}
              >
                <ChevronsRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        )}
      </Card>
    </div>
  )
}

export default AdminDatabases
