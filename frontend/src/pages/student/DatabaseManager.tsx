import React, { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
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
   rows: any[];
   total: number;
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
   const params = useParams<{ id: string }>()
   const navigate = useNavigate()
   const id = projectId || params.id
   const [project, setProject] = useState<Project | null>(null)
   const [activeTab, setActiveTab] = useState('tables')
   const [tables, setTables] = useState<TableInfo[]>([])
   const [selectedTable, setSelectedTable] = useState<string | null>(null)
   const [tableData, setTableData] = useState<TableData | null>(null)
   const [primaryKey, setPrimaryKey] = useState<string | null>(null)
   const [loading, setLoading] = useState(true)
   const [credentials, setCredentials] = useState<DatabaseCredentials | null>(null)

   // Query state
   const [query, setQuery] = useState('')
   const [queryResult, setQueryResult] = useState<any>(null)
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

   useEffect(() => {
      if (id) {
         fetchProject()
         fetchTables()
         fetchCredentials()
      }
   }, [id])

   const fetchProject = async () => {
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
   }

   const fetchCredentials = async () => {
      if (!id) return
      try {
         const res = await databaseAPI.getCredentials(id)
         setCredentials(res.data)
      } catch (err) {
         console.error('Failed to fetch credentials')
      }
   }

   const fetchTables = async () => {
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
   }

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

         const pkCol = structRes.data.columns.find((c: any) => c.key === 'PRI')
         if (pkCol) {
            setPrimaryKey(pkCol.name)
         }
      } catch (err) {
         toast.error(t('common.error'))
      } finally {
         setLoading(false)
      }
   }

   const requestDeleteRow = (row: any) => {
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
            } catch (err: any) {
               toast.error(err.response?.data?.error || t('common.error'));
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
      } catch (err: any) {
         setQueryResult({ error: err.response?.data?.error || t('common.error'), status: 'error' })
         toast.error(err.response?.data?.error || t('common.error'))
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
                     className="mb-4 gap-2 h-8 px-2"
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
                  <Button variant="outline" onClick={() => setShowCredentials(true)} className="gap-2">
                     <Key className="w-4 h-4" /> {t('databaseManager.credentials')}
                  </Button>
                  <Button variant="outline" onClick={confirmReset} className="text-destructive hover:bg-destructive/10 hover:border-destructive/30 gap-2">
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
               <div className="grid grid-cols-1 lg:grid-cols-4 gap-6 h-[600px]">
                  {/* Table Sidebar */}
                  <Card className="lg:col-span-1 flex flex-col overflow-hidden border-none shadow-xl bg-card/95 ring-1 ring-white/5">
                     <CardHeader className="p-6 border-b bg-muted/30">
                        <div className="flex justify-between items-center">
                           <CardTitle className="text-[10px] font-bold uppercase tracking-[0.25em] text-muted-foreground">{t('databaseManager.tableIndex')}</CardTitle>
                           <Button variant="ghost" size="icon" className="h-6 w-6 hover:bg-primary/10 hover:text-primary transition-colors" onClick={fetchTables}>
                              <RefreshCw className={cn("w-3.5 h-3.5", loading && "animate-spin")} />
                           </Button>
                        </div>
                     </CardHeader>
                     <CardContent className="flex-1 overflow-y-auto p-3 space-y-1.5 pt-4">
                        {loading && tables.length === 0 ? (
                           <div className="py-10 flex justify-center"><Loader2 className="w-6 h-6 animate-spin text-primary/30" /></div>
                        ) : tables.length === 0 ? (
                           <div className="text-center py-20 text-muted-foreground font-bold uppercase tracking-widest text-[10px] opacity-40">{t('databaseManager.noTables')}</div>
                        ) : (
                           tables.map(table => (
                              <button
                                 key={table.name}
                                 onClick={() => selectTable(table.name)}
                                 className={cn(
                                    "w-full text-left px-4 py-3 rounded-xl transition-all border text-xs font-bold flex justify-between items-center group",
                                    selectedTable === table.name
                                       ? 'bg-primary/10 border-primary/20 text-primary shadow-sm'
                                       : 'border-transparent text-muted-foreground/70 hover:bg-muted/50 hover:text-foreground'
                                 )}
                              >
                                 <span className="truncate pr-2 tracking-tight">{table.name}</span>
                                 <span className={cn(
                                    "text-[9px] font-mono px-1.5 py-0.5 rounded-md transition-all",
                                    selectedTable === table.name ? 'bg-primary/20 text-primary' : 'bg-muted text-muted-foreground/40 group-hover:bg-muted-foreground/10 group-hover:text-muted-foreground'
                                 )}>{table.rows}</span>
                              </button>
                           ))
                        )}
                     </CardContent>
                  </Card>
 
                  {/* Data Viewer */}
                  <Card className="lg:col-span-3 flex flex-col overflow-hidden border-none shadow-2xl bg-card/95 ring-1 ring-white/5">
                     {selectedTable ? (
                        <>
                           <CardHeader className="p-8 border-b flex flex-row items-center justify-between bg-muted/20">
                              <div className="flex items-center gap-4">
                                 <div className="w-12 h-12 bg-primary/10 border border-primary/20 rounded-2xl flex items-center justify-center text-primary shadow-inner">
                                    <Layers className="w-6 h-6" />
                                 </div>
                                 <div>
                                    <CardTitle className="text-xl font-black tracking-tight text-foreground/90 uppercase">{selectedTable}</CardTitle>
                                    <CardDescription className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground/60">{t('databaseManager.rows')}: {tableData?.total || 0}</CardDescription>
                                 </div>
                              </div>
                              {!primaryKey && (
                                 <Badge variant="outline" className="text-[10px] font-black uppercase tracking-widest border-primary/20 bg-primary/5 text-primary px-3 py-1">{t('databaseManager.readOnly')}</Badge>
                              )}
                           </CardHeader>
 
                           <CardContent className="flex-1 overflow-auto p-0 scrollbar-thin will-change-transform">
                              {loading ? (
                                 <div className="flex items-center justify-center h-96"><Loader2 className="w-10 h-10 animate-spin text-primary/30" /></div>
                              ) : tableData && tableData.rows?.length > 0 ? (
                                 <div className="w-full">
                                    <table className="w-full border-collapse text-left text-xs">
                                       <thead>
                                      <tr className="bg-muted/40 border-b border-border/50">
                                         {tableData.columns?.map((col: string) => (
                                            <th key={col} className="px-8 py-4 font-bold text-[10px] uppercase tracking-[0.2em] text-muted-foreground/80 border-r border-border/30 last:border-r-0">{col}</th>
                                             ))}
                                             {primaryKey && (
                                                <th className="px-8 py-4 font-bold text-[10px] uppercase tracking-[0.2em] text-muted-foreground/0 w-12">{t('common.actions')}</th>
                                             )}
                                          </tr>
                                       </thead>
                                      <tbody className="divide-y divide-border/30">
                                         {tableData.rows.map((row: any, i: number) => (
                                            <tr key={i} className="group hover:bg-muted/20 transition-colors">
                                               {tableData.columns?.map((col: string) => (
                                                  <td key={col} className="px-8 py-5 font-mono text-[11px] font-medium border-r border-border/30 last:border-r-0 overflow-hidden text-ellipsis whitespace-nowrap max-w-[250px] text-foreground/80">
                                                     {row[col] === null ? <span className="text-muted-foreground/30 italic font-normal">NULL</span> : String(row[col])}
                                                  </td>
                                               ))}
                                               {primaryKey && (
                                                  <td className="px-4 py-2 text-right w-12 border-border/30">
                                                      <Button
                                                         variant="ghost"
                                                         size="icon"
                                                         className="h-9 w-9 text-destructive/40 hover:text-destructive hover:bg-destructive/10 rounded-xl transition-all"
                                                         onClick={() => requestDeleteRow(row)}
                                                      >
                                                         <Trash2 className="w-4 h-4" />
                                                      </Button>
                                                   </td>
                                                )}
                                             </tr>
                                          ))}
                                       </tbody>
                                    </table>
                                 </div>
                              ) : (
                                 <div className="flex flex-col items-center justify-center h-96 gap-6 opacity-20">
                                    <div className="w-20 h-20 rounded-full bg-muted flex items-center justify-center">
                                       <PackageOpen className="w-10 h-10" />
                                    </div>
                                    <p className="text-xs font-bold uppercase tracking-[0.3em]">{t('databaseManager.emptyTable')}</p>
                                 </div>
                              )}
                           </CardContent>
                        </>
                     ) : (
                        <CardContent className="flex-1 flex flex-col items-center justify-center text-muted-foreground gap-6 opacity-30 h-96">
                           <div className="w-20 h-20 rounded-full bg-muted flex items-center justify-center animate-pulse">
                              <MousePointer2 className="w-10 h-10" />
                           </div>
                           <p className="text-xs font-bold uppercase tracking-[0.3em]">{t('databaseManager.selectToView')}</p>
                        </CardContent>
                     )}
                  </Card>
               </div>
            </TabsContent>
 
            <TabsContent value="query" className="flex-1 space-y-6">
               <div className="grid grid-cols-1 gap-6 h-auto">
                  <Card className="flex flex-col overflow-hidden bg-zinc-950 dark:bg-zinc-950 border-zinc-200 dark:border-zinc-800 shadow-2xl">
                     <CardHeader className="py-4 px-8 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900/50 flex flex-row items-center justify-between">
                        <div className="flex items-center gap-4">
                           <div className="w-8 h-8 rounded-lg bg-emerald-500/10 flex items-center justify-center">
                              <Terminal className="w-4 h-4 text-emerald-500" />
                           </div>
                           <CardTitle className="text-[10px] font-black text-zinc-500 dark:text-zinc-400 uppercase tracking-[0.25em]">{t('databaseManager.sqlWorkspace')}</CardTitle>
                        </div>
                        <div className="flex items-center gap-3">
                           <Button variant="ghost" size="sm" onClick={() => setQuery('')} className="text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 uppercase font-bold text-[10px] tracking-widest">{t('common.cancel').split(' ')[0]}</Button>
                           <Button
                              size="sm"
                              onClick={executeQuery}
                              disabled={queryLoading || !query.trim()}
                              className="h-9 px-6 bg-emerald-600 hover:bg-emerald-500 text-white font-bold rounded-xl shadow-lg shadow-emerald-900/20 transition-all active:scale-95"
                           >
                              {queryLoading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-3 h-3 mr-2 fill-current" />}
                              {t('projectDetail.actions.execute')}
                           </Button>
                        </div>
                     </CardHeader>
                     <Textarea
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        placeholder="SELECT * FROM users WHERE active = 1;"
                        className="flex-1 min-h-[200px] bg-transparent text-emerald-600 dark:text-emerald-400 font-mono text-base p-8 focus-visible:ring-0 resize-none border-none placeholder:text-zinc-300 dark:placeholder:text-zinc-800 scrollbar-thin transition-colors"
                        spellCheck={false}
                     />
                  </Card>
 
                    <Card className="flex flex-col overflow-hidden border border-border/50 shadow-sm bg-card/95">
                      <CardHeader className="py-4 px-8 bg-muted/30 border-b border-border/50 flex flex-row items-center justify-between">
                        <CardTitle className="text-[10px] font-black uppercase tracking-[0.25em] text-muted-foreground">{t('databaseManager.outputLog')}</CardTitle>
                        {queryResult && (
                           <Badge variant="secondary" className="text-[9px] font-black uppercase tracking-widest bg-muted/50 border-border/30">{t('databaseManager.queryDuration', { ms: queryResult.duration || '0ms' })}</Badge>
                        )}
                     </CardHeader>
                     <CardContent className="flex-1 overflow-auto p-0 scrollbar-thin min-h-[200px]">
                        {queryResult ? (
                           <div className="min-w-full">
                              {queryResult.status === 'error' ? (
                                 <div className="p-8 text-red-500 font-mono text-xs whitespace-pre-wrap leading-relaxed border-l-4 border-red-500 bg-red-500/5">
                                    <div className="font-black text-red-600 mb-3 uppercase tracking-[0.2em] flex items-center gap-2">
                                       <Trash2 className="w-4 h-4" />
                                       [QUERY EXECUTION FAILED]
                                    </div>
                                    <span className="opacity-80">{queryResult.error}</span>
                                 </div>
                              ) : queryResult.rows && queryResult.rows.length > 0 ? (
                                 <table className="w-full text-xs text-left border-collapse">
                                    <thead>
                                    <tr className="bg-muted/40 border-b border-border/50">
                                       {queryResult.columns?.map((col: string) => (<th key={col} className="px-8 py-4 font-bold text-[10px] uppercase tracking-[0.2em] text-muted-foreground/80 border-r border-border/30 last:border-r-0">{col}</th>))}
                                    </tr>
                                 </thead>
                                 <tbody className="divide-y divide-border/30">
                                    {queryResult.rows.map((row: any, i: number) => (
                                       <tr key={i} className="hover:bg-muted/20 transition-colors">
                                          {queryResult.columns?.map((col: string) => (
                                             <td key={col} className="px-8 py-4 font-mono text-[11px] font-medium border-r border-border/30 last:border-r-0 whitespace-nowrap overflow-hidden text-ellipsis max-w-[250px] text-foreground/80">
                                                   {row[col] !== null ? String(row[col]) : <span className="opacity-30 italic font-normal">NULL</span>}
                                                </td>
                                             ))}
                                          </tr>
                                       ))}
                                    </tbody>
                                 </table>
                              ) : (
                                 <div className="p-8 text-emerald-500 font-mono text-xs whitespace-pre-wrap leading-relaxed border-l-4 border-emerald-500 bg-emerald-500/5">
                                    <div className="font-black text-emerald-600 mb-3 uppercase tracking-[0.2em] flex items-center gap-2">
                                       <RefreshCw className="w-4 h-4" />
                                       [QUERY COMPLETED SUCCESSFULLY]
                                    </div>
                                    <span className="opacity-80">{t('databaseManager.transactionComplete', { count: queryResult.rows_affected || 0 })}</span>
                                    <div className="text-muted-foreground/40 mt-6 uppercase tracking-[0.2em] text-[9px] font-bold">
                                       -- SESSION TRACE ID: {Math.random().toString(36).substr(2, 9).toUpperCase()} --
                                    </div>
                                 </div>
                              )}
                           </div>
                        ) : (
                           <div className="h-64 flex flex-col items-center justify-center text-muted-foreground opacity-20 gap-6 transition-opacity">
                              <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center">
                                 <Terminal className="w-8 h-8" />
                              </div>
                              <p className="text-[10px] font-black uppercase tracking-[0.5em]">{t('databaseManager.awaiting')}</p>
                           </div>
                        )}
                     </CardContent>
                  </Card>
               </div>
            </TabsContent>

            <TabsContent value="import" className="space-y-6">
               <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <Card className="hover:border-primary/20 transition-all p-6">
                     <CardContent className="p-0 space-y-6">
                        <div className="w-12 h-12 bg-primary/10 rounded-xl flex items-center justify-center text-primary">
                           <Download className="w-6 h-6" />
                        </div>
                        <div>
                           <CardTitle className="text-xl">{t('databaseManager.backup')}</CardTitle>
                           <CardDescription className="mt-2 text-sm">{t('databaseManager.backupDesc')}</CardDescription>
                        </div>
                        <Button onClick={handleExport} className="w-full gap-2">
                           <Download className="w-4 h-4" />
                           {t('databaseManager.generateDump')}
                        </Button>
                     </CardContent>
                  </Card>

                  <Card className="hover:border-primary/20 transition-all p-6">
                     <CardContent className="p-0 space-y-6">
                        <div className="w-12 h-12 bg-primary/10 rounded-xl flex items-center justify-center text-primary">
                           <Upload className="w-6 h-6" />
                        </div>
                        <div>
                           <CardTitle className="text-xl">{t('databaseManager.import')}</CardTitle>
                           <CardDescription className="mt-2 text-sm">{t('databaseManager.importDesc')}</CardDescription>
                        </div>
                        <Textarea
                           value={importSQL}
                           onChange={(e) => setImportSQL(e.target.value)}
                           placeholder={t('databaseManager.importPlaceholder') || ''}
                           className="h-32 bg-muted/30 font-mono text-xs p-4 focus-visible:ring-primary/20"
                        />
                        <Button
                           onClick={handleImport}
                           disabled={importing || !importSQL.trim()}
                           className="w-full gap-2"
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
                  <DialogTitle className="text-2xl font-bold tracking-tight flex items-center gap-3">
                     <div className="p-2 bg-primary/10 rounded-lg text-primary">
                        <Key className="w-5 h-5" />
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
                           <Button variant="ghost" size="icon" className="h-4 w-4 opacity-0 group-hover:opacity-100 transition-opacity" onClick={() => copyToClipboard(item.label, String(item.value))}>
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
                  <Button className="w-full" onClick={() => setShowCredentials(false)}>{t('databaseManager.closePane')}</Button>
               </DialogFooter>
            </DialogContent>
         </Dialog>
      </div>
   )
}
