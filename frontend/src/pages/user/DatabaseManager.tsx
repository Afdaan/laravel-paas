import React, { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import {
   ArrowLeft,
   Database as DbIcon,
   Key,
   Trash2,
   RefreshCw,
   PackageOpen,
   MousePointer2,
   Play,
   Download,
   Upload,
   Copy,
   Terminal,
   Layers,
   Loader2
} from 'lucide-react'
import { databaseAPI, projectsAPI } from '../../services/api'
import { AxiosError } from 'axios'
import { Project } from '../../types'
import useTranslation from '../../lib/useTranslation'
import ConfirmationModal from '../../components/ConfirmationModal'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

interface TableInfo {
   name: string;
   rows: number;
}

interface TableData {
   columns: string[];
   rows: Record<string, unknown>[];
   total: number;
}

interface QueryResult {
   columns?: string[];
   rows?: Record<string, unknown>[];
   rows_affected?: number;
   duration?: string;
   error?: string;
   status: 'success' | 'error';
}

interface DatabaseCredentials {
   host: string;
   port: number | string;
   database: string;
   username: string;
   password: string;
}

interface DatabaseManagerProps {
   embedded?: boolean;
   projectId?: string | number | null;
}

export default function DatabaseManager({ embedded = false, projectId = null }: DatabaseManagerProps) {
   const { t } = useTranslation()
   const params = useParams<{ uid: string }>()
   const navigate = useNavigate()
   const [searchParams, setSearchParams] = useSearchParams()
   const id = projectId || params.uid
   const [project, setProject] = useState<Project | null>(null)
   const activeTab = searchParams.get('tab') || 'tables'
   const setActiveTab = (tab: string) => {
      setSearchParams(prev => {
         prev.set('tab', tab)
         return prev
      }, { replace: true })
   }
   const [tables, setTables] = useState<TableInfo[]>([])
   const [selectedTable, setSelectedTable] = useState<string | null>(null)
   const [tableData, setTableData] = useState<TableData | null>(null)
   const [primaryKey, setPrimaryKey] = useState<string | null>(null)
   const [loading, setLoading] = useState(true)
   const [credentials, setCredentials] = useState<DatabaseCredentials | null>(null)

   // Query state
   const [query, setQuery] = useState('')
   const [queryResult, setQueryResult] = useState<QueryResult | null>(null)
   const [queryLoading, setQueryLoading] = useState(false)

   // Import state
   const [importSQL, setImportSQL] = useState('')
   const [importing, setImporting] = useState(false)

   // Modal state
   const [showCredentials, setShowCredentials] = useState(false)
   const [confirmModal, setConfirmModal] = useState({
      isOpen: false,
      title: '',
      message: '' as React.ReactNode,
      type: 'danger' as 'danger' | 'warning' | 'info',
      onConfirm: () => { },
      confirmText: 'Confirm'
   })

   const fetchProject = useCallback(async () => {
      if (!id) return
      try {
         const res = await projectsAPI.get(id)
         setProject(res.data)
      } catch (err) {
         if (!embedded) {
            toast.error(t('common.error'))
            navigate('/databases')
         }
      }
   }, [id, embedded, navigate, t])

   const fetchCredentials = useCallback(async () => {
      if (!id) return
      try {
         const res = await databaseAPI.getCredentials(id)
         setCredentials(res.data)
      } catch (err) {
         console.error('Failed to fetch credentials')
      }
   }, [id])

   const fetchTables = useCallback(async () => {
      if (!id) return
      setLoading(true)
      try {
         const res = await databaseAPI.listTables(id)
         setTables(res.data.tables || [])
      } catch (err) {
         toast.error(t('common.error'))
      } finally {
         setLoading(false)
      }
   }, [id, t])

   useEffect(() => {
      if (id) {
         fetchProject()
         fetchTables()
         fetchCredentials()
      }
   }, [id, fetchProject, fetchTables, fetchCredentials])

   const selectTable = async (tableName: string) => {
      if (!id) return
      setSelectedTable(tableName)
      setLoading(true)
      setTableData(null)
      setPrimaryKey(null)
      try {
         const [dataRes, structRes] = await Promise.all([
            databaseAPI.getData(id, tableName, 1, 50),
            databaseAPI.getStructure(id, tableName)
         ])
         setTableData(dataRes.data)

         const pkCol = structRes.data.columns.find((c: { name: string, key: string }) => c.key === 'PRI')
         if (pkCol) {
            setPrimaryKey(pkCol.name)
         }
      } catch (err) {
         toast.error(t('common.error'))
      } finally {
         setLoading(false)
      }
   }

   const requestDeleteRow = (row: Record<string, unknown>) => {
      if (!id || !selectedTable || !primaryKey) return;

      const pkValue = row[primaryKey];

      setConfirmModal({
         isOpen: true,
         title: t('databaseManager.deleteRowConfirm'),
         message: t('databaseManager.deleteRowDesc'),
         type: 'danger',
         confirmText: t('databaseManager.deleteRowAction'),
         onConfirm: async () => {
            try {
               await databaseAPI.deleteRow(id, selectedTable, primaryKey, pkValue);
               toast.success(t('common.success'));
               setConfirmModal(prev => ({ ...prev, isOpen: false }))
               selectTable(selectedTable);
            } catch (err: unknown) {
               const axiosError = err as AxiosError<{ error: string }>
               toast.error(axiosError.response?.data?.error || t('common.error'));
            }
         }
      })
   }

   const executeQuery = async () => {
      if (!id || !query.trim()) return
      setQueryLoading(true)
      try {
         const res = await databaseAPI.query(id, query)
         setQueryResult({ ...res.data, status: 'success' })
         toast.success(t('databaseManager.querySuccess'))
      } catch (err: unknown) {
         const axiosError = err as AxiosError<{ error: string }>
         setQueryResult({ error: axiosError.response?.data?.error || t('common.error'), status: 'error' })
         toast.error(axiosError.response?.data?.error || t('common.error'))
      } finally {
         setQueryLoading(false)
      }
   }

   const handleExport = async () => {
      if (!id) return
      try {
         const res = await databaseAPI.export(id)
         const blob = new Blob([res.data], { type: 'application/sql' })
         const url = window.URL.createObjectURL(blob)
         const a = document.createElement('a')
         a.href = url
         a.download = `${project?.database_name || 'database'}_dump.sql`
         a.click()
         toast.success(t('databaseManager.backupSuccess'))
      } catch (err) {
         toast.error(t('common.error'))
      }
   }

   const handleImport = async () => {
      if (!id || !importSQL.trim()) return
      setImporting(true)
      try {
         await databaseAPI.import(id, importSQL)
         toast.success(t('databaseManager.importSuccess'))
         setImportSQL('')
         fetchTables()
      } catch (err) {
         toast.error(t('common.error'))
      } finally {
         setImporting(false)
      }
   }

   const confirmReset = () => {
      if (!id) return
      setConfirmModal({
         isOpen: true,
         title: t('databaseManager.resetConfirm'),
         message: t('databaseManager.resetDesc'),
         type: 'danger',
         confirmText: t('databaseManager.resetAction'),
         onConfirm: async () => {
            try {
               await databaseAPI.reset(id)
               toast.success(t('common.success'))
               setSelectedTable(null)
               fetchTables()
            } catch (err) {
               toast.error(t('common.error'))
            }
         }
      })
   }

   const copyToClipboard = (label: string, text: string) => {
      navigator.clipboard.writeText(text)
      toast.success(t('databaseManager.copied', { label }))
   }

   return (
      <div className="space-y-6 animate-in fade-in duration-500 h-full flex flex-col">
         <ConfirmationModal
            onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
            {...confirmModal}
         />

         {/* Header Overlay for standalone view */}
         {!embedded && (
            <div className="flex flex-col lg:flex-row lg:items-end justify-between gap-6 pb-6 border-b">
               <div>
                  <Button
                     variant="ghost"
                     size="sm"
                     onClick={() => navigate(-1)}
                     className="mb-4 gap-2 h-8 px-2 cursor-pointer"
                  >
                     <ArrowLeft className="w-4 h-4" />
                     <span className="text-[10px] font-bold uppercase tracking-widest">{t('newProject.back').split(' ')[0]}</span>
                  </Button>
                  <h1 className="text-3xl font-bold tracking-tight flex items-center gap-3">
                     <div className="w-10 h-10 rounded-xl bg-primary/10 flex items-center justify-center text-primary">
                        <DbIcon className="w-5 h-5" />
                     </div>
                     {t('databaseManager.title')}
                  </h1>
                  <p className="text-muted-foreground mt-2 font-mono text-xs uppercase tracking-widest">
                     {t('databaseManager.schema')}: <span className="text-primary font-bold">{project?.database_name}</span>
                  </p>
               </div>
               <div className="flex gap-3">
                  <Button variant="outline" onClick={() => setShowCredentials(true)} className="gap-2 cursor-pointer">
                     <Key className="w-4 h-4" /> {t('databaseManager.credentials')}
                  </Button>
                  <Button
                     variant="outline"
                     onClick={confirmReset}
                     className="text-destructive hover:bg-destructive/10 hover:border-destructive/30 gap-2 cursor-pointer"
                  >
                     <Trash2 className="w-4 h-4" /> {t('databaseManager.reset')}
                  </Button>
               </div>
            </div>
         )}

         {/* Tabs System */}
         <Tabs value={activeTab} onValueChange={setActiveTab} className="flex-1 flex flex-col space-y-6">
            <TabsList className="bg-muted p-1 rounded-lg w-fit">
               <TabsTrigger value="tables" className="gap-2 px-6">
                  <Layers className="w-4 h-4" />
                  {t('databaseManager.tables')}
               </TabsTrigger>
               <TabsTrigger value="query" className="gap-2 px-6">
                  <Terminal className="w-4 h-4" />
                  {t('databaseManager.console')}
               </TabsTrigger>
               <TabsTrigger value="import" className="gap-2 px-6">
                  <RefreshCw className="w-4 h-4" />
                  {t('databaseManager.transfer')}
               </TabsTrigger>
            </TabsList>

            <TabsContent value="tables" className="flex-1 min-h-0">
               <div className="grid grid-cols-1 lg:grid-cols-4 gap-4 h-[600px]">
                  {/* Table Sidebar */}
                  <Card className="lg:col-span-1 flex flex-col overflow-hidden shadow-sm">
                     <CardHeader className="px-4 py-2.5 border-b border-border/40">
                        <div className="flex justify-between items-center">
                           <CardTitle className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">{t('databaseManager.tableIndex')}</CardTitle>
                           <Button variant="ghost" size="icon" className="h-6 w-6 hover:bg-primary/10 hover:text-primary transition-colors cursor-pointer" onClick={fetchTables}>
                              <RefreshCw className={cn("w-3 h-3", loading && "animate-spin")} />
                           </Button>
                        </div>
                     </CardHeader>
                     <CardContent className="flex-1 overflow-y-auto p-1.5 space-y-0.5 scrollbar-thin">
                        {loading && tables.length === 0 ? (
                           <div className="py-10 flex justify-center"><Loader2 className="w-5 h-5 animate-spin text-primary/30" /></div>
                        ) : tables.length === 0 ? (
                           <div className="text-center py-20 text-muted-foreground font-bold uppercase tracking-widest text-[10px] opacity-40">{t('databaseManager.noTables')}</div>
                        ) : (
                           tables.map(table => (
                              <button
                                 key={table.name}
                                 onClick={() => selectTable(table.name)}
                                 className={cn(
                                    "w-full text-left px-3 py-2 rounded-lg transition-all text-xs font-medium flex justify-between items-center group focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 cursor-pointer",
                                    selectedTable === table.name
                                       ? 'bg-primary/10 text-primary font-semibold'
                                       : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
                                 )}
                              >
                                 <span className="truncate pr-2 tracking-tight">{table.name}</span>
                                 <span className={cn(
                                    "text-[9px] font-mono px-1.5 py-0.5 rounded shrink-0 tabular-nums",
                                    selectedTable === table.name ? 'bg-primary/15 text-primary' : 'text-muted-foreground/40 group-hover:text-muted-foreground/70'
                                 )}>{table.rows}</span>
                              </button>
                           ))
                        )}
                     </CardContent>
                  </Card>

                  {/* Data Viewer */}
                  <Card className="lg:col-span-3 flex flex-col overflow-hidden shadow-sm">
                     {selectedTable ? (
                        <>
                           <CardHeader className="px-5 py-3 flex flex-row items-center justify-between border-b border-border/40">
                              <div className="flex items-center gap-3">
                                 <div className="w-8 h-8 bg-primary/10 rounded-lg flex items-center justify-center text-primary">
                                    <Layers className="w-4 h-4" />
                                 </div>
                                 <div>
                                    <CardTitle className="text-sm font-bold tracking-tight text-foreground">{selectedTable}</CardTitle>
                                    <CardDescription className="text-[10px] font-medium text-muted-foreground">{t('databaseManager.rows')}: <span className="font-mono tabular-nums">{tableData?.total || 0}</span></CardDescription>
                                 </div>
                              </div>
                              {!primaryKey && (
                                 <Badge variant="outline" className="text-[9px] font-bold uppercase tracking-widest border-primary/20 bg-primary/5 text-primary px-2 py-0.5 h-5">{t('databaseManager.readOnly')}</Badge>
                              )}
                           </CardHeader>

                           <CardContent className="flex-1 overflow-auto p-0 scrollbar-thin will-change-transform">
                              {loading ? (
                                 <div className="flex items-center justify-center h-96"><Loader2 className="w-6 h-6 animate-spin text-primary/30" /></div>
                              ) : tableData && tableData.rows?.length > 0 ? (
                                 <div className="w-full">
                                    <table className="w-full border-collapse text-left text-xs">
                                       <thead>
                                          <tr className="bg-muted/30 border-b border-border/50 sticky top-0">
                                             {tableData.columns?.map((col: string) => (
                                                <th key={col} className="px-4 py-2.5 font-semibold text-[10px] uppercase tracking-wider text-muted-foreground border-r border-border/20 last:border-r-0">{col}</th>
                                             ))}
                                             {primaryKey && (
                                                <th className="px-4 py-2.5 font-semibold text-[10px] uppercase tracking-wider text-transparent w-10">{t('common.actions')}</th>
                                             )}
                                          </tr>
                                       </thead>
                                       <tbody className="divide-y divide-border/20">
                                          {tableData.rows.map((row, i: number) => (
                                             <tr key={i} className="group hover:bg-muted/15 transition-colors">
                                                {tableData.columns?.map((col: string) => (
                                                   <td key={col} className="px-4 py-3 font-mono text-[11px] font-normal border-r border-border/20 last:border-r-0 overflow-hidden text-ellipsis whitespace-nowrap max-w-[220px] text-foreground/75">
                                                      {row[col] === null ? <span className="text-muted-foreground/30 italic">NULL</span> : String(row[col])}
                                                   </td>
                                                ))}
                                                {primaryKey && (
                                                   <td className="px-2 py-1.5 text-center w-10">
                                                      <Button
                                                         variant="ghost"
                                                         size="icon"
                                                         className="h-7 w-7 text-destructive/30 hover:text-destructive hover:bg-destructive/10 rounded-lg transition-all cursor-pointer opacity-0 group-hover:opacity-100"
                                                         onClick={() => requestDeleteRow(row)}
                                                      >
                                                         <Trash2 className="w-3.5 h-3.5" />
                                                      </Button>
                                                   </td>
                                                )}
                                             </tr>
                                          ))}
                                       </tbody>
                                    </table>
                                 </div>
                              ) : (
                                 <div className="flex flex-col items-center justify-center h-96 gap-4 opacity-20">
                                    <div className="w-14 h-14 rounded-xl bg-muted flex items-center justify-center">
                                       <PackageOpen className="w-7 h-7" />
                                    </div>
                                    <p className="text-[10px] font-bold uppercase tracking-[0.3em]">{t('databaseManager.emptyTable')}</p>
                                 </div>
                              )}
                           </CardContent>
                        </>
                     ) : (
                        <CardContent className="flex-1 flex flex-col items-center justify-center text-muted-foreground gap-4 opacity-25 h-96">
                           <div className="w-14 h-14 rounded-xl bg-muted flex items-center justify-center">
                              <MousePointer2 className="w-7 h-7" />
                           </div>
                           <p className="text-[10px] font-bold uppercase tracking-[0.3em]">{t('databaseManager.selectToView')}</p>
                        </CardContent>
                     )}
                  </Card>
               </div>
            </TabsContent>

            <TabsContent value="query" className="flex-1 space-y-4">
               <div className="grid grid-cols-1 gap-4 h-auto">
                  <Card className="flex flex-col overflow-hidden shadow-sm">
                     <CardHeader className="py-2.5 px-5 border-b border-border/40 flex flex-row items-center justify-between">
                        <div className="flex items-center gap-3">
                           <div className="w-7 h-7 rounded-md bg-muted flex items-center justify-center">
                              <Terminal className="w-3.5 h-3.5 text-muted-foreground" />
                           </div>
                           <CardTitle className="text-xs font-semibold text-muted-foreground">{t('databaseManager.sqlWorkspace')}</CardTitle>
                        </div>
                        <div className="flex items-center gap-2">
                           <Button variant="ghost" size="sm" onClick={() => setQuery('')} className="text-muted-foreground hover:text-foreground text-xs cursor-pointer">{t('common.cancel').split(' ')[0]}</Button>
                           <Button
                              size="sm"
                              onClick={executeQuery}
                              disabled={queryLoading || !query.trim()}
                              className="gap-1.5 cursor-pointer"
                           >
                              {queryLoading ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Play className="w-3 h-3 fill-current" />}
                              {t('projectDetail.actions.execute')}
                           </Button>
                        </div>
                     </CardHeader>
                     <Textarea
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        placeholder="SELECT * FROM users WHERE active = 1;"
                        className="flex-1 min-h-[180px] bg-muted/30 text-foreground font-mono text-sm p-5 focus-visible:ring-0 resize-none border-none placeholder:text-muted-foreground/30 scrollbar-thin transition-colors"
                        spellCheck={false}
                     />
                  </Card>

                  <Card className="flex flex-col overflow-hidden shadow-sm">
                     <CardHeader className="py-2.5 px-5 border-b border-border/40 flex flex-row items-center justify-between">
                        <CardTitle className="text-xs font-semibold text-muted-foreground">{t('databaseManager.outputLog')}</CardTitle>
                        {queryResult && (
                           <Badge variant="secondary" className="text-[10px] font-medium">{t('databaseManager.queryDuration', { ms: queryResult.duration || '0ms' })}</Badge>
                        )}
                     </CardHeader>
                     <CardContent className="flex-1 overflow-auto p-0 scrollbar-thin min-h-[180px]">
                        {queryResult ? (
                           <div className="min-w-full">
                              {queryResult.status === 'error' ? (
                                 <div className="p-5 font-mono text-xs whitespace-pre-wrap leading-relaxed">
                                    <div className="flex items-center gap-2 text-rose-500 font-semibold mb-2 text-[11px]">
                                       <Trash2 className="w-3.5 h-3.5 shrink-0" />
                                       {t('databaseManager.queryFailed') || 'Query execution failed'}
                                    </div>
                                    <span className="text-rose-400/80">{queryResult.error}</span>
                                 </div>
                              ) : queryResult.rows && queryResult.rows.length > 0 ? (
                                 <table className="w-full text-xs text-left border-collapse">
                                    <thead>
                                       <tr className="bg-muted/30 border-b border-border/50">
                                          {queryResult.columns?.map((col: string) => (<th key={col} className="px-4 py-2.5 font-semibold text-[11px] text-muted-foreground border-r border-border/20 last:border-r-0">{col}</th>))}
                                       </tr>
                                    </thead>
                                    <tbody className="divide-y divide-border/20">
                                       {queryResult.rows.map((row, i: number) => (
                                          <tr key={i} className="hover:bg-muted/20 transition-colors">
                                             {queryResult.columns?.map((col: string) => (
                                                <td key={col} className="px-4 py-2.5 font-mono text-[11px] border-r border-border/20 last:border-r-0 whitespace-nowrap overflow-hidden text-ellipsis max-w-[250px] text-foreground/80">
                                                   {row[col] !== null ? String(row[col]) : <span className="text-muted-foreground/40 italic">NULL</span>}
                                                </td>
                                             ))}
                                          </tr>
                                       ))}
                                    </tbody>
                                 </table>
                              ) : (
                                 <div className="p-5 font-mono text-xs leading-relaxed">
                                    <div className="flex items-center gap-2 text-emerald-500 font-semibold mb-1.5 text-[11px]">
                                       <RefreshCw className="w-3.5 h-3.5 shrink-0" />
                                       {t('databaseManager.querySuccess') || 'Query completed successfully'}
                                    </div>
                                    <span className="text-muted-foreground">{t('databaseManager.transactionComplete', { count: queryResult.rows_affected || 0 })}</span>
                                 </div>
                              )}
                           </div>
                        ) : (
                           <div className="h-48 flex flex-col items-center justify-center text-muted-foreground/30 gap-3">
                              <div className="w-10 h-10 rounded-lg bg-muted/50 flex items-center justify-center">
                                 <Terminal className="w-5 h-5" />
                              </div>
                              <p className="text-xs font-medium">{t('databaseManager.awaiting')}</p>
                           </div>
                        )}
                     </CardContent>
                  </Card>
               </div>
            </TabsContent>

            <TabsContent value="import" className="space-y-4">
               <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <Card className="hover:border-primary/20 transition-colors p-5">
                     <CardContent className="p-0 space-y-4">
                        <div className="w-8 h-8 bg-primary/10 rounded-lg flex items-center justify-center text-primary">
                           <Download className="w-4 h-4" />
                        </div>
                        <div>
                           <CardTitle className="text-sm">{t('databaseManager.backup')}</CardTitle>
                           <CardDescription className="mt-1 text-xs">{t('databaseManager.backupDesc')}</CardDescription>
                        </div>
                        <Button onClick={handleExport} className="w-full gap-2 cursor-pointer">
                           <Download className="w-4 h-4" />
                           {t('databaseManager.generateDump')}
                        </Button>
                     </CardContent>
                  </Card>

                  <Card className="hover:border-primary/20 transition-colors p-5">
                     <CardContent className="p-0 space-y-4">
                        <div className="w-8 h-8 bg-primary/10 rounded-lg flex items-center justify-center text-primary">
                           <Upload className="w-4 h-4" />
                        </div>
                        <div>
                           <CardTitle className="text-sm">{t('databaseManager.import')}</CardTitle>
                           <CardDescription className="mt-1 text-xs">{t('databaseManager.importDesc')}</CardDescription>
                        </div>
                        <Textarea
                           value={importSQL}
                           onChange={(e) => setImportSQL(e.target.value)}
                           placeholder={t('databaseManager.importPlaceholder') || ''}
                           className="h-28 bg-muted/30 font-mono text-xs p-3 focus-visible:ring-primary/20"
                        />
                        <Button
                           onClick={handleImport}
                           disabled={importing || !importSQL.trim()}
                           className="w-full gap-2 cursor-pointer"
                        >
                           {importing ? <Loader2 className="w-4 h-4 animate-spin" /> : <Upload className="w-4 h-4" />}
                           {t('databaseManager.runImport')}
                        </Button>
                     </CardContent>
                  </Card>
               </div>
            </TabsContent>
         </Tabs>

         {/* Credentials Dialog */}
         <Dialog open={showCredentials} onOpenChange={setShowCredentials}>
            <DialogContent className="sm:max-w-md">
               <DialogHeader>
                  <DialogTitle className="text-base font-semibold flex items-center gap-2.5">
                     <div className="p-1.5 bg-primary/10 rounded-md text-primary">
                        <Key className="w-4 h-4" />
                     </div>
                     {t('databaseManager.credsTitle')}
                  </DialogTitle>
                  <DialogDescription>
                     {t('databaseManager.credsDesc')}
                  </DialogDescription>
               </DialogHeader>

               <div className="space-y-4 my-2">
                  {[
                     { label: t('databaseManager.networkHost'), value: credentials?.host || '...' },
                     { label: t('databaseManager.port'), value: credentials?.port || '3306' },
                     { label: t('databaseManager.dbName'), value: credentials?.database || '...' },
                     { label: t('databaseManager.userName'), value: credentials?.username || '...' },
                     { label: t('databaseManager.password'), value: credentials?.password || '...', secret: true },
                  ].map(item => (
                     <div key={item.label} className="space-y-1.5 p-3 rounded-lg bg-muted/50 border group hover:border-primary/20 transition-colors">
                        <div className="flex justify-between items-center">
                           <Label className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">{item.label}</Label>
                           <Button variant="ghost" size="icon" className="h-4 w-4 opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer" onClick={() => copyToClipboard(item.label, String(item.value))}>
                              <Copy className="w-3 h-3" />
                           </Button>
                        </div>
                        <div className="font-mono text-xs font-bold flex items-center justify-between">
                           {item.secret ? "••••••••••••••••" : item.value}
                        </div>
                     </div>
                  ))}
               </div>

               <DialogFooter>
                  <Button className="w-full cursor-pointer" onClick={() => setShowCredentials(false)}>{t('databaseManager.closePane')}</Button>
               </DialogFooter>
            </DialogContent>
         </Dialog>
      </div>
   )
}
