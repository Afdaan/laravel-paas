import React, { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { databaseAPI, projectsAPI } from '../../services/api'
import { DatabaseInstance, DatabaseBackup } from '../../types'
import {
  Database,
  RefreshCw,
  Key,
  Shield,
  ShieldAlert,
  History,
  Trash2,
  Terminal,
  Plus,
  Play,
  Copy,
  Eye,
  EyeOff,
  Server,
  Activity,
  HardDrive,
  Table,
  PlusCircle,
  AlertTriangle,
  DatabaseZap,
  Info
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger } from '@/components/ui/select'
import { cn } from '@/lib/utils'

function DatabaseStudio() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [activeTab, setActiveTab] = useState<'overview' | 'viewer' | 'designer' | 'scratchpad' | 'backups'>('overview')
  
  const [isLoading, setIsLoading] = useState(true)
  const [isActionLoading, setIsActionLoading] = useState(false)
  
  // Data states
  const [dbOverview, setDbOverview] = useState<any>(null)
  const [schemaData, setSchemaData] = useState<any[]>([])
  const [backups, setBackups] = useState<DatabaseBackup[]>([])
  const [metrics, setMetrics] = useState<any>(null)
  
  // Credentials reveal toggles
  const [revealPassword, setRevealPassword] = useState(false)
  
  // SQL Scratchpad states
  const [sqlQuery, setSqlQuery] = useState('SELECT * FROM users LIMIT 10;')
  const [queryResult, setQueryResult] = useState<any>(null)
  const [queryHistory, setQueryHistory] = useState<string[]>([])
  
  // Table Viewer states
  const [selectedTable, setSelectedTable] = useState<string>('')
  const [tableData, setTableData] = useState<any>(null)
  const [tablePage, setTablePage] = useState(1)
  const [tableLimit] = useState(25)
  const [tableTotal, setTableTotal] = useState(0)
  
  // Visual Designer states
  const [designerAction, setDesignerAction] = useState<'create_table' | 'add_column' | 'create_index' | null>(null)
  const [newTableName, setNewTableName] = useState('')
  const [newColName, setNewColName] = useState('')
  const [newColType, setNewColType] = useState('varchar')
  const [newColLength, setNewColLength] = useState(255)
  const [newColNullable, setNewColNullable] = useState(true)
  
  const [indexName, setIndexName] = useState('')
  const [indexCols, setIndexCols] = useState<string[]>([])

  // Load complete studio dataset
  const loadStudioData = useCallback(async () => {
    if (!id) return
    setIsLoading(true)
    try {
      const overviewRes = await databaseAPI.getOverview(id)
      setDbOverview(overviewRes.data)
      
      const schemaRes = await databaseAPI.getSchema(id)
      setSchemaData(schemaRes.data.tables || [])
      if (schemaRes.data.tables && schemaRes.data.tables.length > 0) {
        setSelectedTable(schemaRes.data.tables[0].name)
      }
      
      const backupsRes = await databaseAPI.listBackups(id)
      setBackups(backupsRes.data.backups || [])
      
      const metricsRes = await databaseAPI.getMetrics(id)
      setMetrics(metricsRes.data)
    } catch (err: any) {
      toast.error('Failed to connect to Managed Database. Verify project is running.')
    } finally {
      setIsLoading(false)
    }
  }, [id])

  useEffect(() => {
    loadStudioData()
  }, [loadStudioData])

  // Load paginated table data in data grid
  const loadTableDataGrid = useCallback(async () => {
    if (!id || !selectedTable) return
    try {
      const res = await databaseAPI.getData(id, selectedTable, tablePage, tableLimit)
      setTableData({
        columns: res.data.columns || [],
        rows: res.data.rows || []
      })
      setTableTotal(res.data.total || 0)
    } catch (err: any) {
      toast.error('Failed to read table rows.')
    }
  }, [id, selectedTable, tablePage, tableLimit])

  useEffect(() => {
    if (activeTab === 'viewer' && selectedTable) {
      loadTableDataGrid()
    }
  }, [activeTab, selectedTable, tablePage, loadTableDataGrid])

  // Action helpers
  const handleRotateCredentials = async () => {
    if (!id) return
    if (!window.confirm('Are you absolutely sure you want to rotate credentials? This will trigger an instant, zero-downtime hot-swap environment restart.')) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.rotateCredentials(id)
      toast.success(res.data.message)
      loadStudioData()
    } catch (err: any) {
      toast.error('Failed to rotate credentials.')
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleRestartPool = async () => {
    if (!id) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.restartDatabase(id)
      toast.success(res.data.message)
    } catch (err: any) {
      toast.error('Connection restart test failed: ' + (err.response?.data?.error || err.message))
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleToggleStatus = async (suspend: boolean) => {
    if (!id) return
    const msg = suspend 
      ? 'Suspend database? This revokes connect privileges and forcefully terminates all active tenant connections immediately.'
      : 'Resume database? This restores connection access.'
    if (!window.confirm(msg)) return
    
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.updateStatus(id, suspend)
      toast.success(res.data.message)
      loadStudioData()
    } catch (err: any) {
      toast.error('Failed to update status.')
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleExecuteSQL = async () => {
    if (!id || !sqlQuery.trim()) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.query(id, sqlQuery)
      setQueryResult(res.data)
      setQueryHistory(prev => [sqlQuery, ...prev.slice(0, 9)])
      toast.success('Query executed successfully.')
    } catch (err: any) {
      const errMsg = err.response?.data?.error || err.message
      setQueryResult({ error: errMsg })
      toast.error('Query execution failed: ' + errMsg)
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleCreateBackup = async () => {
    if (!id) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.createBackup(id)
      toast.success(res.data.message)
      // Reload backups list
      const bRes = await databaseAPI.listBackups(id)
      setBackups(bRes.data.backups || [])
    } catch (err: any) {
      toast.error('Failed to generate snapshot backup.')
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleRestoreBackup = async (backupId: number) => {
    if (!id) return
    if (!window.confirm('WARNING: Restoring this backup will drop all existing tables and overwrite database state completely. Proceed?')) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.restoreBackup(id, backupId)
      toast.success(res.data.message)
      loadStudioData()
    } catch (err: any) {
      toast.error('Failed to restore snapshot.')
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleDeleteBackup = async (backupId: number) => {
    if (!id) return
    if (!window.confirm('Delete this backup snapshot? This is permanent.')) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.deleteBackup(id, backupId)
      toast.success(res.data.message)
      setBackups(prev => prev.filter(b => b.id !== backupId))
    } catch (err: any) {
      toast.error('Failed to prune backup.')
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleDeleteRow = async (row: any, pkCol: string) => {
    if (!id || !selectedTable) return
    if (!window.confirm('Are you sure you want to visually delete this row?')) return
    try {
      const res = await databaseAPI.deleteRow(id, selectedTable, pkCol, row[pkCol])
      toast.success('Row deleted securely.')
      loadTableDataGrid()
    } catch (err: any) {
      toast.error('Failed to delete row.')
    }
  }

  const handleDesignerAction = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!id) return
    
    let payload: any = {
      action: designerAction,
      table_name: selectedTable || newTableName
    }

    if (designerAction === 'create_table') {
      payload.table_name = newTableName
      payload.column = {
        name: 'id',
        type: 'integer',
        primary_key: true,
        nullable: false
      }
    } else if (designerAction === 'add_column') {
      payload.column = {
        name: newColName,
        type: newColType,
        length: newColLength,
        nullable: newColNullable
      }
    } else if (designerAction === 'create_index') {
      payload.index_name = indexName
      payload.index_columns = indexCols
    }

    setIsActionLoading(true)
    try {
      await databaseAPI.executeDesigner(id, payload)
      toast.success('Table structure updated.')
      setDesignerAction(null)
      loadStudioData()
    } catch (err: any) {
      toast.error(err.response?.data?.error || 'Visual designer action failed.')
    } finally {
      setIsActionLoading(false)
    }
  }

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text)
    toast.success(`${label} copied to clipboard!`)
  }

  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center min-h-[500px] gap-4">
        <LoaderSpinner className="w-12 h-12 text-primary animate-spin" />
        <p className="text-muted-foreground text-sm font-semibold tracking-wide uppercase">Connecting to Managed Database Studio...</p>
      </div>
    )
  }

  const instanceStatus = dbOverview?.status || 'active'
  const isSuspended = instanceStatus === 'suspended'

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-20">
      {/* Studio Header */}
      <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-4 border-b border-border/40 pb-6">
        <div className="space-y-1.5">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-primary/10 flex items-center justify-center text-primary border border-primary/20">
              <Database className="w-5 h-5" />
            </div>
            <div>
              <h1 className="text-3xl font-extrabold tracking-tight">Database <span className="text-primary italic">Studio</span></h1>
              <p className="text-muted-foreground text-xs uppercase tracking-widest font-bold">Isolated Tenant Resources & DDL Schema Designer</p>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-3 shrink-0">
          <Button
            variant="outline"
            size="sm"
            onClick={loadStudioData}
            disabled={isActionLoading}
            className="gap-2 h-10"
          >
            <RefreshCw className={cn("w-4 h-4", isActionLoading && "animate-spin")} />
            Sync State
          </Button>

          {isSuspended ? (
            <Button
              variant="default"
              size="sm"
              onClick={() => handleToggleStatus(false)}
              disabled={isActionLoading}
              className="gap-2 h-10 bg-emerald-600 hover:bg-emerald-700 text-white font-bold"
            >
              <Shield className="w-4 h-4" />
              Resume DB Instance
            </Button>
          ) : (
            <Button
              variant="destructive"
              size="sm"
              onClick={() => handleToggleStatus(true)}
              disabled={isActionLoading}
              className="gap-2 h-10 font-bold"
            >
              <ShieldAlert className="w-4 h-4" />
              Suspend DB
            </Button>
          )}
        </div>
      </div>

      {/* Tab Navigation */}
      <div className="flex border-b border-border/60 p-1 bg-muted/20 rounded-xl w-fit">
        {(['overview', 'viewer', 'designer', 'scratchpad', 'backups'] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={cn(
              "px-5 py-2.5 rounded-lg text-sm font-bold capitalize transition-all duration-200",
              activeTab === tab 
                ? "bg-background text-primary shadow-sm border border-border/40" 
                : "text-muted-foreground hover:text-foreground"
            )}
          >
            {tab}
          </button>
        ))}
      </div>

      {/* Database Warning if Suspended */}
      {isSuspended && (
        <Card className="border-destructive/30 bg-destructive/5 p-5 animate-in slide-in-from-top-2 duration-300">
          <div className="flex items-start gap-4">
            <div className="p-2.5 bg-destructive/10 rounded-lg text-destructive shrink-0">
              <AlertTriangle className="w-5 h-5" />
            </div>
            <div>
              <h4 className="font-extrabold uppercase tracking-wide text-destructive text-sm">Database Instance Suspended</h4>
              <p className="text-muted-foreground text-xs mt-1 leading-relaxed">
                Connect privileges have been actively revoked and all active backend connections have been forcefully terminated to secure resources. Restart or resume the instance to restore full data access.
              </p>
            </div>
          </div>
        </Card>
      )}

      {/* Tab Contents */}
      {activeTab === 'overview' && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          {/* Left / Main Overview */}
          <div className="lg:col-span-2 space-y-8">
            {/* Metric Grid */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-5">
              <Card className="p-5 flex flex-col gap-1 hover:border-primary/20 transition-all duration-300">
                <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">Engine</span>
                <span className="text-lg font-extrabold text-foreground capitalize flex items-center gap-1.5">
                  <Server className="w-4 h-4 text-primary" />
                  {dbOverview?.engine || 'MySQL'}
                </span>
              </Card>

              <Card className="p-5 flex flex-col gap-1 hover:border-primary/20 transition-all duration-300">
                <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">Version</span>
                <span className="text-xs font-mono font-bold truncate mt-1 text-foreground" title={dbOverview?.version}>
                  {dbOverview?.version || 'Unknown'}
                </span>
              </Card>

              <Card className="p-5 flex flex-col gap-1 hover:border-primary/20 transition-all duration-300">
                <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">Disk Used</span>
                <span className="text-lg font-extrabold text-foreground flex items-center gap-1.5">
                  <HardDrive className="w-4 h-4 text-primary" />
                  {dbOverview?.size || '0 KB'}
                </span>
              </Card>

              <Card className="p-5 flex flex-col gap-1 hover:border-primary/20 transition-all duration-300">
                <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">Tables</span>
                <span className="text-lg font-extrabold text-foreground flex items-center gap-1.5">
                  <Table className="w-4 h-4 text-primary" />
                  {dbOverview?.table_count || 0}
                </span>
              </Card>
            </div>

            {/* Connection Metrics */}
            {metrics && (
              <Card className="p-6">
                <h3 className="font-extrabold text-base mb-4 flex items-center gap-2">
                  <Activity className="w-5 h-5 text-primary" />
                  Real-time Connection Performance
                </h3>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <div className="space-y-2">
                    <div className="flex justify-between text-xs font-bold uppercase tracking-wider text-muted-foreground">
                      <span>Concurrent Connections</span>
                      <span>{metrics.active_connections} / 15 connections (Limit)</span>
                    </div>
                    <div className="h-2 w-full bg-muted rounded-full overflow-hidden">
                      <div 
                        className="h-full bg-primary transition-all duration-500" 
                        style={{ width: `${Math.min((metrics.active_connections / 15) * 100, 100)}%` }}
                      />
                    </div>
                    <p className="text-[10px] text-muted-foreground italic">Strict 15-connection ceiling is enforced per tenant to guarantee cluster DoS protection.</p>
                  </div>

                  <div className="space-y-2">
                    <div className="flex justify-between text-xs font-bold uppercase tracking-wider text-muted-foreground">
                      <span>Disk Allocation</span>
                      <span>{dbOverview?.size || '0 KB'} / 1 GB (Quota)</span>
                    </div>
                    <div className="h-2 w-full bg-muted rounded-full overflow-hidden">
                      <div 
                        className="h-full bg-primary transition-all duration-500" 
                        style={{ width: `${Math.min((metrics.size_kb / 1048576) * 100, 100)}%` }}
                      />
                    </div>
                    <p className="text-[10px] text-muted-foreground italic">Managed volumes prevent shared disk space starvation.</p>
                  </div>
                </div>
              </Card>
            )}

            {/* Connection Credentials Card */}
            <Card className="p-6">
              <div className="flex items-center justify-between border-b pb-4 mb-5">
                <h3 className="font-extrabold text-base flex items-center gap-2">
                  <Key className="w-5 h-5 text-primary" />
                  Connection Credentials
                </h3>
                <Button
                  variant="outline"
                  size="xs"
                  onClick={handleRotateCredentials}
                  disabled={isActionLoading || isSuspended}
                  className="font-bold border-primary/20 hover:border-primary shrink-0 gap-1.5 h-8 text-xs"
                >
                  <RefreshCw className="w-3.5 h-3.5" />
                  Rotate Password
                </Button>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
                <div className="space-y-1.5">
                  <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Host</span>
                  <div className="flex items-center justify-between p-3 rounded-lg border bg-muted/10 font-mono text-xs">
                    <span className="truncate">{dbOverview?.host || 'localhost'}</span>
                    <button onClick={() => copyToClipboard(dbOverview?.host || '', 'Host')} className="text-muted-foreground hover:text-foreground">
                      <Copy className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Port</span>
                  <div className="flex items-center justify-between p-3 rounded-lg border bg-muted/10 font-mono text-xs">
                    <span>{dbOverview?.port || 3306}</span>
                    <button onClick={() => copyToClipboard(String(dbOverview?.port || 3306), 'Port')} className="text-muted-foreground hover:text-foreground">
                      <Copy className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Database Name</span>
                  <div className="flex items-center justify-between p-3 rounded-lg border bg-muted/10 font-mono text-xs">
                    <span className="truncate">{dbOverview?.database || ''}</span>
                    <button onClick={() => copyToClipboard(dbOverview?.database || '', 'Database Name')} className="text-muted-foreground hover:text-foreground">
                      <Copy className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Username</span>
                  <div className="flex items-center justify-between p-3 rounded-lg border bg-muted/10 font-mono text-xs">
                    <span className="truncate">{dbOverview?.username || ''}</span>
                    <button onClick={() => copyToClipboard(dbOverview?.username || '', 'Username')} className="text-muted-foreground hover:text-foreground">
                      <Copy className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>

                <div className="space-y-1.5 md:col-span-2">
                  <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Password</span>
                  <div className="flex items-center justify-between p-3 rounded-lg border bg-muted/10 font-mono text-xs">
                    <input 
                      type={revealPassword ? "text" : "password"} 
                      value={dbOverview?.password || ''} 
                      readOnly 
                      className="bg-transparent border-none outline-none focus:ring-0 flex-1 min-w-0 pr-4"
                    />
                    <div className="flex items-center gap-3 shrink-0">
                      <button onClick={() => setRevealPassword(!revealPassword)} className="text-muted-foreground hover:text-foreground">
                        {revealPassword ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                      </button>
                      <button onClick={() => copyToClipboard(dbOverview?.password || '', 'Password')} className="text-muted-foreground hover:text-foreground">
                        <Copy className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </Card>
          </div>

          {/* Right Sidebar - Admin Controls / SRE Info */}
          <div className="space-y-8">
            <Card className="p-5 bg-muted/5 border-primary/10">
              <h4 className="font-extrabold text-sm mb-3 flex items-center gap-2 text-primary uppercase tracking-wide border-b pb-2">
                <Shield className="w-4.5 h-4.5" />
                SRE Tenant Isolation
              </h4>
              <ul className="space-y-3.5 pl-1.5">
                <li className="text-xs text-muted-foreground flex items-start gap-2.5">
                  <span className="w-1.5 h-1.5 bg-primary rounded-full shrink-0 mt-1.5" />
                  <span><strong>Port Mapping:</strong> isolated PostgreSQL listens on host port <code>5433</code>, isolated MySQL on <code>3306</code>.</span>
                </li>
                <li className="text-xs text-muted-foreground flex items-start gap-2.5">
                  <span className="w-1.5 h-1.5 bg-primary rounded-full shrink-0 mt-1.5" />
                  <span><strong>DoS Cap:</strong> Enforces connection throttling limit of 15 simultaneous database threads per tenant.</span>
                </li>
                <li className="text-xs text-muted-foreground flex items-start gap-2.5">
                  <span className="w-1.5 h-1.5 bg-primary rounded-full shrink-0 mt-1.5" />
                  <span><strong>Query Timeouts:</strong> All raw SQL queries are evaluated under a strict 15-second contextual CPU execution timeout.</span>
                </li>
                <li className="text-xs text-muted-foreground flex items-start gap-2.5">
                  <span className="w-1.5 h-1.5 bg-primary rounded-full shrink-0 mt-1.5" />
                  <span><strong>Audit Logs:</strong> Visual DDL designer updates write tamper-proof audit trails mapping user IP addresses.</span>
                </li>
              </ul>
            </Card>

            <Card className="p-5 space-y-4">
              <h4 className="font-extrabold text-sm uppercase tracking-wide border-b pb-2 flex items-center gap-2">
                <Activity className="w-4.5 h-4.5" />
                Instance Status & Actions
              </h4>
              
              <div className="space-y-3">
                <div className="flex justify-between items-center text-xs">
                  <span className="font-bold text-muted-foreground uppercase tracking-wider">Status:</span>
                  <span className={cn(
                    "px-2.5 py-0.5 rounded-full text-[10px] font-extrabold uppercase tracking-wide border",
                    isSuspended 
                      ? "bg-destructive/10 text-destructive border-destructive/20 animate-pulse" 
                      : "bg-emerald-500/10 text-emerald-500 border-emerald-500/20"
                  )}>
                    {instanceStatus}
                  </span>
                </div>

                <div className="space-y-2 border-t pt-3">
                  <Button
                    variant="outline"
                    className="w-full text-xs font-bold gap-2 hover:bg-muted"
                    onClick={handleRestartPool}
                    disabled={isActionLoading || isSuspended}
                  >
                    <RefreshCw className="w-3.5 h-3.5" />
                    Verify Connection Pool
                  </Button>
                </div>
              </div>
            </Card>
          </div>
        </div>
      )}

      {activeTab === 'viewer' && (
        <Card className="p-6">
          <div className="flex flex-col md:flex-row items-start md:items-center justify-between border-b pb-4 mb-5 gap-4">
            <div className="flex items-center gap-3">
              <Table className="w-5 h-5 text-primary" />
              <div>
                <h3 className="font-extrabold text-base">Live Table Data Grid</h3>
                <p className="text-muted-foreground text-xs">Browse records visually and prune rows securely without SQL</p>
              </div>
            </div>

            {schemaData.length > 0 && (
              <Select
                value={selectedTable}
                onValueChange={(val) => {
                  setSelectedTable(val)
                  setTablePage(1)
                }}
              >
                <SelectTrigger className="w-56 h-10 border-border bg-background/50 hover:bg-background/80 text-sm font-semibold rounded-xl">
                  <span>{selectedTable || 'Select Table'}</span>
                </SelectTrigger>
                <SelectContent className="bg-popover border border-border rounded-xl shadow-2xl p-1.5 max-h-60">
                  {schemaData.map(t => (
                    <SelectItem key={t.name} value={t.name} className="rounded-lg py-2.5 px-3 cursor-pointer">
                      {t.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>

          {isSuspended ? (
            <div className="py-12 text-center text-muted-foreground text-sm font-semibold uppercase tracking-wide">
              No database access. Database is suspended.
            </div>
          ) : !selectedTable ? (
            <div className="py-12 border border-dashed rounded-xl flex flex-col items-center justify-center text-center gap-3 bg-muted/5">
              <Table className="w-8 h-8 text-muted-foreground" />
              <div className="space-y-1">
                <h4 className="font-bold text-sm">No Tables Found</h4>
                <p className="text-xs text-muted-foreground">Use the Visual Designer tab to build your first database table.</p>
              </div>
            </div>
          ) : tableData ? (
            <div className="space-y-4">
              <div className="overflow-x-auto border border-border/80 rounded-xl bg-background/30 max-h-[500px]">
                <table className="w-full text-left border-collapse text-xs font-medium">
                  <thead>
                    <tr className="bg-muted/30 border-b border-border/80 text-[10px] font-bold uppercase tracking-widest text-muted-foreground">
                      <th className="py-3.5 px-4 w-12 text-center">Action</th>
                      {tableData.columns.map((col: string) => (
                        <th key={col} className="py-3.5 px-4 font-mono font-semibold">{col}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {tableData.rows.length === 0 ? (
                      <tr>
                        <td colSpan={tableData.columns.length + 1} className="py-10 text-center text-muted-foreground italic font-semibold">
                          Table is empty. No rows exist.
                        </td>
                      </tr>
                    ) : (
                      tableData.rows.map((row: any, idx: number) => {
                        // Dynamically look for common primary key column names (id, uid, uuid)
                        const pkColumn = tableData.columns.find((c: string) => 
                          c.toLowerCase() === 'id' || c.toLowerCase() === 'uid' || c.toLowerCase() === 'uuid'
                        ) || tableData.columns[0]

                        return (
                          <tr key={idx} className="border-b border-border/40 hover:bg-muted/15 transition-colors">
                            <td className="py-3.5 px-4 text-center shrink-0">
                              <button 
                                onClick={() => handleDeleteRow(row, pkColumn)}
                                className="p-1 bg-destructive/10 hover:bg-destructive/20 text-destructive rounded transition-colors"
                              >
                                <Trash2 className="w-3.5 h-3.5" />
                              </button>
                            </td>
                            {tableData.columns.map((col: string) => (
                              <td key={col} className="py-3.5 px-4 font-mono whitespace-nowrap overflow-hidden text-ellipsis max-w-[200px]" title={String(row[col] ?? '')}>
                                {row[col] === null ? <span className="text-muted-foreground/30 italic">NULL</span> : String(row[col])}
                              </td>
                            ))}
                          </tr>
                        )
                      })
                    )}
                  </tbody>
                </table>
              </div>

              {/* Data Grid Pagination */}
              {tableTotal > tableLimit && (
                <div className="flex items-center justify-between border-t border-border/40 pt-4">
                  <span className="text-xs text-muted-foreground font-semibold">
                    Showing {(tablePage - 1) * tableLimit + 1} - {Math.min(tablePage * tableLimit, tableTotal)} of {tableTotal} rows
                  </span>
                  <div className="flex items-center gap-2">
                    <Button 
                      variant="outline" 
                      size="xs" 
                      onClick={() => setTablePage(prev => Math.max(prev - 1, 1))}
                      disabled={tablePage === 1}
                      className="font-bold h-8 text-xs px-3 rounded-lg"
                    >
                      Prev
                    </Button>
                    <Button 
                      variant="outline" 
                      size="xs" 
                      onClick={() => setTablePage(prev => prev + 1)}
                      disabled={tablePage * tableLimit >= tableTotal}
                      className="font-bold h-8 text-xs px-3 rounded-lg"
                    >
                      Next
                    </Button>
                  </div>
                </div>
              )}
            </div>
          ) : (
            <div className="py-10 text-center text-muted-foreground">Loading Table Rows...</div>
          )}
        </Card>
      )}

      {activeTab === 'designer' && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          {/* Main Visual Schema Explorer */}
          <div className="lg:col-span-2 space-y-6">
            <Card className="p-6">
              <div className="flex items-center justify-between border-b pb-4 mb-5">
                <div>
                  <h3 className="font-extrabold text-base">Visual Table Designer GUI</h3>
                  <p className="text-muted-foreground text-xs">Direct visual schema architecting, writes audit log markers</p>
                </div>
                
                <Button
                  size="sm"
                  onClick={() => setDesignerAction('create_table')}
                  disabled={isActionLoading || isSuspended}
                  className="font-bold shrink-0 gap-1.5 h-10 rounded-xl"
                >
                  <PlusCircle className="w-4 h-4" />
                  Create Table
                </Button>
              </div>

              {isSuspended ? (
                <div className="py-12 text-center text-muted-foreground text-sm font-semibold uppercase tracking-wide">
                  No visual designer access. Database is suspended.
                </div>
              ) : schemaData.length === 0 ? (
                <div className="py-16 text-center border border-dashed rounded-xl flex flex-col items-center justify-center gap-4 bg-muted/5">
                  <DatabaseZap className="w-10 h-10 text-muted-foreground/60" />
                  <div className="space-y-1">
                    <h4 className="font-extrabold text-base">Database contains no schema objects</h4>
                    <p className="text-xs text-muted-foreground max-w-sm">Table schemas have not been declared. Click the Create Table trigger to begin visual schema modeling.</p>
                  </div>
                </div>
              ) : (
                <div className="space-y-6">
                  {schemaData.map((table: any) => (
                    <div key={table.name} className="border border-border/80 rounded-xl overflow-hidden shadow-sm bg-background/10">
                      <div className="flex items-center justify-between p-4 bg-muted/20 border-b border-border/80">
                        <span className="font-mono font-bold text-sm text-foreground/90 flex items-center gap-2">
                          <Table className="w-4 h-4 text-primary" />
                          {table.name}
                        </span>
                        
                        <div className="flex items-center gap-2">
                          <Button
                            variant="outline"
                            size="xs"
                            onClick={() => {
                              setSelectedTable(table.name)
                              setDesignerAction('add_column')
                            }}
                            className="h-8 text-xs font-bold gap-1 rounded-lg"
                          >
                            <Plus className="w-3.5 h-3.5" />
                            Add Column
                          </Button>
                        </div>
                      </div>

                      <div className="overflow-x-auto">
                        <table className="w-full text-left border-collapse text-xs font-medium">
                          <thead>
                            <tr className="border-b border-border/40 bg-muted/5 text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                              <th className="py-3 px-4">Column</th>
                              <th className="py-3 px-4">Datatype</th>
                              <th className="py-3 px-4 text-center">Nullable</th>
                              <th className="py-3 px-4 text-center">Keys</th>
                              <th className="py-3 px-4">Default</th>
                            </tr>
                          </thead>
                          <tbody>
                            {table.columns.map((col: any) => (
                              <tr key={col.name} className="border-b border-border/20 hover:bg-muted/5 transition-colors">
                                <td className="py-3 px-4 font-mono font-semibold text-foreground/90">{col.name}</td>
                                <td className="py-3 px-4 font-mono text-primary/80">{col.type}</td>
                                <td className="py-3 px-4 text-center">
                                  <span className={cn(
                                    "px-2 py-0.5 rounded text-[10px] font-bold border",
                                    col.nullable 
                                      ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/20" 
                                      : "bg-amber-500/10 text-amber-500 border-amber-500/20"
                                  )}>
                                    {col.nullable ? 'YES' : 'NO'}
                                  </span>
                                </td>
                                <td className="py-3 px-4 text-center">
                                  {col.key === 'PRI' && (
                                    <span className="px-2 py-0.5 rounded bg-primary/10 text-primary border border-primary/20 text-[10px] font-extrabold uppercase tracking-wide">
                                      PK
                                    </span>
                                  )}
                                </td>
                                <td className="py-3 px-4 font-mono text-muted-foreground">{col.default === null ? <span className="text-muted-foreground/30 italic">NULL</span> : String(col.default)}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </Card>
          </div>

          {/* Designer Interactive Modals/Panels */}
          <div className="space-y-6">
            {designerAction === 'create_table' && (
              <Card className="p-5 border-primary/20 bg-primary/5 animate-in slide-in-from-right-3 duration-300">
                <h4 className="font-extrabold text-sm mb-4 uppercase tracking-wide text-primary flex items-center gap-1.5">
                  <Table className="w-4.5 h-4.5" />
                  Create Table Model
                </h4>
                <form onSubmit={handleDesignerAction} className="space-y-4">
                  <div className="space-y-1.5">
                    <Label htmlFor="new_table_name" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Table Name</Label>
                    <Input
                      id="new_table_name"
                      value={newTableName}
                      onChange={(e) => setNewTableName(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ''))}
                      placeholder="e.g. posts"
                      required
                      className="h-10 rounded-xl"
                    />
                  </div>
                  <p className="text-[10px] text-muted-foreground italic leading-relaxed">Automatic design: GORM structures require a primary key. An auto-incrementing integer key <code>id</code> will be added automatically.</p>
                  
                  <div className="flex gap-2.5 pt-2">
                    <Button type="submit" disabled={isActionLoading} size="sm" className="font-bold flex-1 rounded-xl">
                      Execute Alter
                    </Button>
                    <Button type="button" onClick={() => setDesignerAction(null)} variant="outline" size="sm" className="font-bold flex-1 rounded-xl">
                      Cancel
                    </Button>
                  </div>
                </form>
              </Card>
            )}

            {designerAction === 'add_column' && (
              <Card className="p-5 border-primary/20 bg-primary/5 animate-in slide-in-from-right-3 duration-300">
                <h4 className="font-extrabold text-sm mb-4 uppercase tracking-wide text-primary flex items-center gap-1.5">
                  <PlusCircle className="w-4.5 h-4.5" />
                  Add Column Model — {selectedTable}
                </h4>
                <form onSubmit={handleDesignerAction} className="space-y-4">
                  <div className="space-y-1.5">
                    <Label htmlFor="new_col_name" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Column Name</Label>
                    <Input
                      id="new_col_name"
                      value={newColName}
                      onChange={(e) => setNewColName(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ''))}
                      placeholder="e.g. title"
                      required
                      className="h-10 rounded-xl"
                    />
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <div className="space-y-1.5">
                      <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Type</Label>
                      <select
                        value={newColType}
                        onChange={(e) => setNewColType(e.target.value)}
                        className="w-full h-10 px-3 rounded-xl border border-border bg-background text-xs font-semibold"
                      >
                        <option value="varchar">VARCHAR</option>
                        <option value="integer">INTEGER</option>
                        <option value="bigint">BIGINT</option>
                        <option value="text">TEXT</option>
                        <option value="boolean">BOOLEAN</option>
                        <option value="timestamp">TIMESTAMP</option>
                      </select>
                    </div>

                    {newColType === 'varchar' && (
                      <div className="space-y-1.5">
                        <Label htmlFor="new_col_len" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Length</Label>
                        <Input
                          id="new_col_len"
                          type="number"
                          value={newColLength}
                          onChange={(e) => setNewColLength(Number(e.target.value))}
                          placeholder="255"
                          className="h-10 rounded-xl text-xs"
                        />
                      </div>
                    )}
                  </div>

                  <div className="flex items-center justify-between border-t pt-3 mt-4">
                    <Label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">Nullable</Label>
                    <button
                      type="button"
                      onClick={() => setNewColNullable(!newColNullable)}
                      className={cn(
                        "relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out",
                        newColNullable ? "bg-primary" : "bg-muted"
                      )}
                    >
                      <span
                        className={cn(
                          "pointer-events-none inline-block h-4 w-4 transform rounded-full bg-background shadow ring-0 transition duration-200 ease-in-out",
                          newColNullable ? "translate-x-4" : "translate-x-0"
                        )}
                      />
                    </button>
                  </div>

                  <div className="flex gap-2.5 pt-2">
                    <Button type="submit" disabled={isActionLoading} size="sm" className="font-bold flex-1 rounded-xl">
                      Add Field
                    </Button>
                    <Button type="button" onClick={() => setDesignerAction(null)} variant="outline" size="sm" className="font-bold flex-1 rounded-xl">
                      Cancel
                    </Button>
                  </div>
                </form>
              </Card>
            )}

            <Card className="p-5 bg-muted/5 border-primary/10">
              <h4 className="font-extrabold text-sm mb-3 flex items-center gap-2 text-primary uppercase tracking-wide border-b pb-2">
                <Info className="w-4.5 h-4.5" />
                Designer Guidelines
              </h4>
              <p className="text-xs text-muted-foreground leading-relaxed pl-1">
                Visual migrations automatically map and format standard SQL statements. All updates generated are instantly evaluated. GORM schema reconciliation triggers automatically to synchronize ORM metadata tables safely.
              </p>
            </Card>
          </div>
        </div>
      )}

      {activeTab === 'scratchpad' && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          {/* Main SQL Area */}
          <div className="lg:col-span-2 space-y-6">
            <Card className="p-6">
              <div className="flex items-center justify-between border-b pb-4 mb-4">
                <div>
                  <h3 className="font-extrabold text-base flex items-center gap-2">
                    <Terminal className="w-5 h-5 text-primary" />
                    SQL Scratchpad Workspace
                  </h3>
                  <p className="text-muted-foreground text-xs">Execute raw statements directly, protected by a 15-second contextual timeout</p>
                </div>

                <Button
                  onClick={handleExecuteSQL}
                  disabled={isActionLoading || isSuspended || !sqlQuery.trim()}
                  className="font-bold shrink-0 gap-1.5 h-10 rounded-xl"
                >
                  <Play className="w-4 h-4 fill-current" />
                  Execute Query
                </Button>
              </div>

              {isSuspended ? (
                <div className="py-12 text-center text-muted-foreground text-sm font-semibold uppercase tracking-wide">
                  No sql access. Database is suspended.
                </div>
              ) : (
                <div className="space-y-4">
                  <div className="border border-border/85 rounded-xl overflow-hidden focus-within:border-primary/50 transition-all">
                    <textarea
                      value={sqlQuery}
                      onChange={(e) => setSqlQuery(e.target.value)}
                      className="w-full h-44 p-4 font-mono text-xs bg-background/50 border-none outline-none focus:ring-0 leading-relaxed resize-y"
                      placeholder="SELECT * FROM table LIMIT 10;"
                    />
                  </div>

                  {/* SQL Execution Result Grid */}
                  {queryResult && (
                    <div className="border border-border/80 rounded-xl overflow-hidden bg-background/10 animate-in zoom-in-95">
                      <div className="flex justify-between items-center bg-muted/30 px-4 py-3 border-b border-border/80 text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                        <span>Query Execution Output</span>
                        {queryResult.duration && <span>Duration: {queryResult.duration}</span>}
                      </div>

                      {queryResult.error ? (
                        <div className="p-4 text-xs font-mono text-destructive bg-destructive/5 font-semibold">
                          Error: {queryResult.error}
                        </div>
                      ) : queryResult.columns && queryResult.columns.length > 0 ? (
                        <div className="overflow-x-auto max-h-[350px]">
                          <table className="w-full text-left border-collapse text-xs font-medium">
                            <thead>
                              <tr className="bg-muted/10 border-b border-border/40 text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                                {queryResult.columns.map((col: string) => (
                                  <th key={col} className="py-3 px-4 font-mono font-semibold">{col}</th>
                                ))}
                              </tr>
                            </thead>
                            <tbody>
                              {queryResult.rows && queryResult.rows.length === 0 ? (
                                <tr>
                                  <td colSpan={queryResult.columns.length} className="py-8 text-center text-muted-foreground italic font-semibold">
                                    No records matched query.
                                  </td>
                                </tr>
                              ) : (
                                queryResult.rows && queryResult.rows.map((row: any, rIdx: number) => (
                                  <tr key={rIdx} className="border-b border-border/20 hover:bg-muted/5 transition-colors">
                                    {queryResult.columns.map((col: string) => (
                                      <td key={col} className="py-3 px-4 font-mono whitespace-nowrap overflow-hidden text-ellipsis max-w-[200px]" title={String(row[col] ?? '')}>
                                        {row[col] === null ? <span className="text-muted-foreground/30 italic">NULL</span> : String(row[col])}
                                      </td>
                                    ))}
                                  </tr>
                                ))
                              )}
                            </tbody>
                          </table>
                        </div>
                      ) : (
                        <div className="p-4 text-xs font-mono text-muted-foreground font-semibold">
                          Success. Rows affected: {queryResult.rows_affected}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}
            </Card>
          </div>

          {/* Right History Sidebar */}
          <div className="space-y-6">
            <Card className="p-5">
              <h4 className="font-extrabold text-sm mb-4 border-b pb-2 flex items-center gap-2 uppercase tracking-wide">
                <History className="w-4.5 h-4.5" />
                Query History
              </h4>
              
              {queryHistory.length === 0 ? (
                <div className="py-8 text-center text-xs text-muted-foreground italic font-semibold">
                  No queries run in this session.
                </div>
              ) : (
                <div className="space-y-3.5 max-h-[350px] overflow-y-auto">
                  {queryHistory.map((q, idx) => (
                    <button
                      key={idx}
                      onClick={() => setSqlQuery(q)}
                      className="w-full text-left p-3 border rounded-xl hover:border-primary/30 font-mono text-[10px] text-muted-foreground hover:text-foreground hover:bg-muted/5 transition-all text-ellipsis overflow-hidden whitespace-nowrap block"
                      title={q}
                    >
                      {q}
                    </button>
                  ))}
                </div>
              )}
            </Card>
          </div>
        </div>
      )}

      {activeTab === 'backups' && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          {/* Backups List */}
          <div className="lg:col-span-2 space-y-6">
            <Card className="p-6">
              <div className="flex items-center justify-between border-b pb-4 mb-5">
                <div>
                  <h3 className="font-extrabold text-base">Point-In-Time backups Snapshot Catalog</h3>
                  <p className="text-muted-foreground text-xs">Cron database backups and automated retention history catalog</p>
                </div>
                
                <Button
                  size="sm"
                  onClick={handleCreateBackup}
                  disabled={isActionLoading || isSuspended}
                  className="font-bold shrink-0 gap-1.5 h-10 rounded-xl"
                >
                  <PlusCircle className="w-4 h-4" />
                  Capture Snapshot
                </Button>
              </div>

              {isSuspended ? (
                <div className="py-12 text-center text-muted-foreground text-sm font-semibold uppercase tracking-wide">
                  No backup access. Database is suspended.
                </div>
              ) : backups.length === 0 ? (
                <div className="py-16 text-center border border-dashed rounded-xl flex flex-col items-center justify-center gap-4 bg-muted/5 animate-in fade-in duration-300">
                  <DatabaseZap className="w-10 h-10 text-muted-foreground/60" />
                  <div className="space-y-1">
                    <h4 className="font-extrabold text-base">No backups archives created</h4>
                    <p className="text-xs text-muted-foreground max-w-sm">Automatic chronological cataloging has not run yet. Take a manual snapshot backup to verify catalog access.</p>
                  </div>
                </div>
              ) : (
                <div className="space-y-4">
                  {backups.map((b) => (
                    <div key={b.id} className="flex flex-col md:flex-row md:items-center justify-between p-4 border border-border/80 rounded-xl bg-background/30 hover:border-primary/15 transition-all duration-300 gap-4">
                      <div className="space-y-1 min-w-0 flex-1">
                        <span className="font-mono font-bold text-xs text-foreground/90 truncate block">{b.name}</span>
                        <div className="flex items-center gap-3.5 text-[10px] text-muted-foreground">
                          <span className="font-semibold uppercase tracking-wider">{b.size}</span>
                          <span className="w-1 h-1 bg-muted-foreground rounded-full" />
                          <span>Captured {new Date(b.created_at).toLocaleString()}</span>
                          <span className="w-1 h-1 bg-muted-foreground rounded-full" />
                          <span className={cn(
                            "px-2 py-0.5 rounded text-[8px] font-extrabold uppercase border",
                            b.status === 'completed' 
                              ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/20" 
                              : b.status === 'pending'
                              ? "bg-amber-500/10 text-amber-500 border-amber-500/20 animate-pulse"
                              : "bg-destructive/10 text-destructive border-destructive/20"
                          )}>
                            {b.status}
                          </span>
                        </div>
                      </div>

                      <div className="flex items-center gap-2.5 shrink-0 justify-end">
                        <Button
                          variant="outline"
                          size="xs"
                          onClick={() => handleRestoreBackup(b.id)}
                          disabled={isActionLoading || b.status !== 'completed'}
                          className="h-8 text-xs font-bold gap-1 rounded-lg hover:border-primary/20 hover:text-primary transition-colors"
                        >
                          <RefreshCw className="w-3.5 h-3.5" />
                          Restore
                        </Button>
                        <Button
                          variant="outline"
                          size="xs"
                          onClick={() => handleDeleteBackup(b.id)}
                          disabled={isActionLoading}
                          className="h-8 text-xs font-bold gap-1 rounded-lg hover:bg-destructive/10 hover:text-destructive hover:border-destructive/30 transition-colors"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                          Prune
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </Card>
          </div>

          {/* SRE Cap Sidebar */}
          <div className="space-y-6">
            <Card className="p-5 border-primary/10 bg-primary/5">
              <h4 className="font-extrabold text-sm mb-3 flex items-center gap-2 text-primary uppercase tracking-wide border-b pb-2">
                <Shield className="w-4.5 h-4.5" />
                SRE Pruning Retention
              </h4>
              <p className="text-xs text-muted-foreground leading-relaxed">
                To prevent VPS hard drive storage starvation, the platform enforces a **strict maximum cap of 5 historical backups** per tenant database instance.
              </p>
              <div className="flex justify-between items-center text-xs mt-4 pt-3 border-t border-primary/10">
                <span className="font-bold text-muted-foreground">Catalog Capacity:</span>
                <span className="font-mono font-bold text-primary">{backups.filter(b => b.status === 'completed').length} / 5 snapshots</span>
              </div>
              <p className="text-[10px] text-muted-foreground italic mt-3 leading-relaxed">
                Creating a 6th snapshot automatically deletes the oldest completed backup archive.
              </p>
            </Card>
          </div>
        </div>
      )}
    </div>
  )
}

function LoaderSpinner({ className }: { className?: string }) {
  return (
    <svg className={className} xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
    </svg>
  )
}

export default DatabaseStudio
