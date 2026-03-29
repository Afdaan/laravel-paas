// ===========================================
// Database Manager Component
// ===========================================

import { useState, useEffect } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'
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
  X,
  Terminal,
  Layers,
  Search,
  Check,
  AlertCircle
} from 'lucide-react'
import { databaseAPI, projectsAPI } from '../../services/api'
import ConfirmationModal from '../../components/ConfirmationModal'

export default function DatabaseManager({ embedded = false, projectId = null }) {
  const params = useParams()
  const navigate = useNavigate()
  const id = projectId || params.id
  const [project, setProject] = useState(null)
  const [activeTab, setActiveTab] = useState('tables')
  const [tables, setTables] = useState([])
  const [selectedTable, setSelectedTable] = useState(null)
  const [tableData, setTableData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [credentials, setCredentials] = useState(null)
  
  // Query state
  const [query, setQuery] = useState('')
  const [queryResult, setQueryResult] = useState(null)
  const [queryLoading, setQueryLoading] = useState(false)
  
  // Import state
  const [importSQL, setImportSQL] = useState('')
  const [importing, setImporting] = useState(false)
  
  // Modal state
  const [showCredentials, setShowCredentials] = useState(false)
  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '',
    type: 'danger',
    onConfirm: () => {},
    confirmText: 'Confirm'
  })

  useEffect(() => {
    fetchProject()
    fetchTables()
    fetchCredentials()
  }, [id])

  const fetchProject = async () => {
    try {
      const res = await projectsAPI.get(id)
      setProject(res.data)
    } catch (err) {
      if (!embedded) {
        toast.error('Failed to load project context')
        navigate('/databases')
      }
    }
  }

  const fetchCredentials = async () => {
    try {
      const res = await databaseAPI.getCredentials(id)
      setCredentials(res.data)
    } catch (err) {
      console.error('Failed to fetch credentials')
    }
  }

  const fetchTables = async () => {
    setLoading(true)
    try {
      const res = await databaseAPI.listTables(id)
      setTables(res.data.tables || [])
    } catch (err) {
      toast.error('Failed to fetch system tables')
    } finally {
      setLoading(false)
    }
  }

  const selectTable = async (tableName) => {
    setSelectedTable(tableName)
    setLoading(true)
    setTableData(null)
    try {
      const res = await databaseAPI.getData(id, tableName, 1, 50)
      setTableData(res.data)
    } catch (err) {
      toast.error('Failed to stream table data')
    } finally {
      setLoading(false)
    }
  }

  const executeQuery = async () => {
    if (!query.trim()) return
    setQueryLoading(true)
    try {
      const res = await databaseAPI.query(id, query)
      setQueryResult(res.data)
      toast.success('Query Executed Successfully')
    } catch (err) {
      toast.error(err.response?.data?.error || 'Database operation failed')
    } finally {
      setQueryLoading(false)
    }
  }

  const handleExport = async () => {
    try {
      const res = await databaseAPI.export(id)
      const blob = new Blob([res.data], { type: 'application/sql' })
      const url = window.URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${project?.database_name || 'database'}_dump.sql`
      a.click()
      toast.success('System Dump Generated')
    } catch (err) {
      toast.error('Export Failed')
    }
  }

  const handleImport = async () => {
    if (!importSQL.trim()) return
    setImporting(true)
    try {
      await databaseAPI.import(id, importSQL)
      toast.success('Data Import Successful')
      setImportSQL('')
      fetchTables()
    } catch (err) {
      toast.error('Import Failed')
    } finally {
      setImporting(false)
    }
  }

  const confirmReset = () => {
    setConfirmModal({
      isOpen: true,
      title: 'Initialize Reset?',
      message: 'This will purge all data clusters and destroy every table in this database instance.',
      type: 'danger',
      confirmText: 'Execute Wipe',
      onConfirm: async () => {
        try {
          await databaseAPI.reset(id)
          toast.success('Database Purged')
          setSelectedTable(null)
          fetchTables()
        } catch (err) {
          toast.error('Purge Failed')
        }
      }
    })
  }

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text)
    toast.success('Credentials Copied', {
      style: { background: '#111114', color: '#fff' }
    })
  }

  const tabs = [
    { id: 'tables', label: 'Schema', icon: Layers },
    { id: 'query', label: 'Console', icon: Terminal },
    { id: 'import', label: 'Sync', icon: RefreshCw },
  ]

  return (
    <div className="space-y-8 animate-pop-in h-full flex flex-col">
      <ConfirmationModal 
        isOpen={confirmModal.isOpen}
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
      />

      {/* Embedded Header Control */}
      {!embedded && (
        <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 pb-6 border-b border-white/5">
          <div>
            <button onClick={() => navigate(-1)} className="flex items-center gap-2 text-slate-500 hover:text-white transition-colors mb-4 group">
              <ArrowLeft className="w-4 h-4 group-hover:-translate-x-1 transition-transform" />
              <span className="text-[10px] font-black uppercase tracking-widest">Return to Cluster</span>
            </button>
            <h1 className="text-4xl font-black text-white tracking-tighter flex items-center gap-3">
              <div className="w-12 h-12 rounded-2xl bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
                <DbIcon className="w-6 h-6" />
              </div>
              Data Terminal
            </h1>
            <p className="text-slate-500 mt-2 font-mono text-xs uppercase tracking-widest">
              Context: <span className="text-indigo-400">{project?.database_name}</span>
            </p>
          </div>
          <div className="flex gap-3">
            <button onClick={() => setShowCredentials(true)} className="btn btn-secondary py-2.5 px-5 text-sm">
              <Key className="w-4 h-4" /> Credentials
            </button>
            <button onClick={confirmReset} className="btn bg-rose-500/10 border border-rose-500/20 text-rose-400 hover:bg-rose-500/20 px-5 text-sm">
              <Trash2 className="w-4 h-4" /> Purge DB
            </button>
          </div>
        </div>
      )}

      {/* Tabs System */}
      <div className={`${embedded ? 'h-full flex flex-col' : ''}`}>
        <div className="flex gap-2 bg-white/[0.03] border border-white/5 p-1.5 rounded-2xl w-fit mb-8 ml-2">
          {tabs.map(tab => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2.5 px-6 py-2.5 rounded-xl text-xs font-black uppercase tracking-widest transition-all duration-300 ${
                activeTab === tab.id 
                ? 'bg-indigo-500 text-white shadow-lg shadow-indigo-500/20' 
                : 'text-slate-400 hover:text-white hover:bg-white/5'
              }`}
            >
              <tab.icon className="w-4 h-4" />
              {tab.label}
            </button>
          ))}
        </div>

        {/* Content Area */}
        <div className="flex-1 overflow-hidden">
          {activeTab === 'tables' && (
            <div className="grid grid-cols-1 lg:grid-cols-4 gap-8 h-[600px]">
              {/* Table Sidebar */}
              <div className="lg:col-span-1 flex flex-col h-full card-glass p-0 overflow-hidden border-white/10">
                <div className="p-6 border-b border-white/5 flex justify-between items-center bg-white/[0.02]">
                  <h3 className="text-[10px] font-black text-white uppercase tracking-[0.2em]">Data Entities</h3>
                  <button onClick={fetchTables} className="text-slate-500 hover:text-indigo-400 transition-colors">
                    <RefreshCw className="w-4 h-4" />
                  </button>
                </div>
                <div className="flex-1 overflow-y-auto p-4 space-y-2">
                  {loading && tables.length === 0 ? (
                    <div className="py-10 flex justify-center"><RefreshCw className="w-6 h-6 animate-spin text-slate-700" /></div>
                  ) : tables.length === 0 ? (
                    <div className="text-center py-20 text-slate-600 font-bold uppercase tracking-widest text-[10px]">Registry Empty</div>
                  ) : (
                    tables.map(table => (
                      <button
                        key={table.name}
                        onClick={() => selectTable(table.name)}
                        className={`w-full text-left p-4 rounded-xl transition-all duration-300 border flex justify-between items-center group uppercase ${
                          selectedTable === table.name 
                            ? 'bg-indigo-500/10 border-indigo-500/30 text-indigo-400' 
                            : 'border-transparent text-slate-400 hover:bg-white/[0.03] hover:border-white/5'
                        }`}
                      >
                        <span className="text-xs font-black tracking-tight truncate">{table.name}</span>
                        <span className="text-[10px] opacity-40 font-mono italic group-hover:opacity-100">{table.rows}</span>
                      </button>
                    ))
                  )}
                </div>
              </div>

              {/* Data Viewer */}
              <div className="lg:col-span-3 flex flex-col h-full card-glass p-0 overflow-hidden border-white/10 bg-white/[0.01]">
                {selectedTable ? (
                  <>
                    <div className="p-6 border-b border-white/5 bg-white/[0.03] flex justify-between items-center">
                      <div className="flex items-center gap-4">
                        <div className="w-10 h-10 rounded-xl bg-indigo-500/5 border border-white/5 flex items-center justify-center text-indigo-400">
                          <Layers className="w-5 h-5" />
                        </div>
                        <div>
                          <h3 className="text-lg font-black text-white tracking-tight uppercase">{selectedTable}</h3>
                          <p className="text-[10px] font-black text-indigo-400/50 uppercase tracking-widest">Entity Rows: {tableData?.total || 0}</p>
                        </div>
                      </div>
                    </div>
                    
                    <div className="flex-1 overflow-auto table-container">
                      {loading ? (
                        <div className="flex items-center justify-center h-full"><RefreshCw className="w-8 h-8 animate-spin text-slate-800" /></div>
                      ) : tableData && tableData.rows?.length > 0 ? (
                        <table className="premium-table">
                          <thead>
                            <tr>
                              {tableData.columns?.map(col => (
                                <th key={col}>{col}</th>
                              ))}
                            </tr>
                          </thead>
                          <tbody>
                            {tableData.rows.map((row, i) => (
                              <tr key={i}>
                                {tableData.columns?.map(col => (
                                  <td key={col}>
                                    {row[col] === null ? <span className="text-slate-700 italic">NULL</span> : String(row[col])}
                                  </td>
                                ))}
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      ) : (
                        <div className="flex flex-col items-center justify-center h-full gap-6 opacity-30">
                          <PackageOpen className="w-16 h-16" />
                          <p className="text-xs font-black uppercase tracking-[0.2em]">Entity is void</p>
                        </div>
                      )}
                    </div>
                  </>
                ) : (
                  <div className="flex flex-col items-center justify-center h-full text-slate-600 gap-6 opacity-40 animate-pulse">
                    <MousePointer2 className="w-12 h-12" />
                    <p className="text-xs font-black uppercase tracking-[0.3em]">Select entity to stream data</p>
                  </div>
                )}
              </div>
            </div>
          )}

          {activeTab === 'query' && (
            <div className="h-[600px] flex flex-col gap-8 animate-pop-in">
              <div className="flex-1 card p-0 overflow-hidden flex flex-col border-white/10 bg-black shadow-inner">
                <div className="px-6 py-4 bg-white/[0.02] border-b border-white/5 flex justify-between items-center">
                   <div className="flex items-center gap-3">
                     <Terminal className="w-4 h-4 text-emerald-500" />
                     <h3 className="text-[10px] font-black text-white uppercase tracking-[0.2em]">Power Console</h3>
                   </div>
                   <div className="flex gap-3">
                      <button onClick={() => setQuery('')} className="text-[10px] font-black text-slate-500 hover:text-white uppercase tracking-widest transition-colors">Clear</button>
                      <button 
                        onClick={executeQuery}
                        disabled={queryLoading || !query.trim()}
                        className="btn btn-primary py-2 px-4 shadow-emerald-500/10 disabled:opacity-50"
                      >
                        {queryLoading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
                        Execute
                      </button>
                   </div>
                </div>
                <textarea
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="-- ENTER SQL COMMANDS HERE..."
                  className="flex-1 bg-transparent text-emerald-400 font-mono text-sm p-8 focus:outline-none resize-none placeholder:text-slate-800"
                  spellCheck="false"
                />
              </div>

              <div className="flex-1 card-glass p-0 overflow-hidden flex flex-col border-white/10">
                 <div className="px-6 py-4 bg-white/[0.02] border-b border-white/5 flex items-center justify-between">
                   <h3 className="text-[10px] font-black text-white uppercase tracking-[0.2em]">Execution Stream</h3>
                   {queryResult && (
                     <span className="text-[10px] font-black text-indigo-400 uppercase tracking-widest">Operation time: {queryResult.duration}</span>
                   )}
                 </div>
                 <div className="flex-1 overflow-auto bg-black/40">
                    {queryResult ? (
                      <div className="p-0">
                        {queryResult.rows && queryResult.rows.length > 0 ? (
                           <table className="premium-table">
                            <thead>
                              <tr>
                                {queryResult.columns?.map(col => (<th key={col}>{col}</th>))}
                              </tr>
                            </thead>
                            <tbody>
                              {queryResult.rows.map((row, i) => (
                                <tr key={i}>
                                  {queryResult.columns?.map(col => (
                                    <td key={col}>{row[col] !== null ? String(row[col]) : <span className="text-slate-800">NULL</span>}</td>
                                  ))}
                                </tr>
                              ))}
                            </tbody>
                           </table>
                        ) : (
                          <div className="p-10 flex flex-col items-center justify-center gap-4">
                            <Check className="w-12 h-12 text-emerald-500" />
                            <p className="text-xs font-black text-emerald-500 uppercase tracking-widest">
                              Command Successful: {queryResult.rows_affected} rows transformed
                            </p>
                          </div>
                        )}
                      </div>
                    ) : (
                      <div className="h-full flex items-center justify-center text-slate-700 italic text-xs font-black uppercase tracking-[0.3em] opacity-40">
                        Awaiting Command Execution
                      </div>
                    )}
                 </div>
              </div>
            </div>
          )}

          {activeTab === 'import' && (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-8 animate-pop-in">
              <div className="card-glass border-white/10 p-10 space-y-6">
                <div className="w-16 h-16 rounded-2xl bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400 mb-2">
                  <Download className="w-8 h-8" />
                </div>
                <h3 className="text-2xl font-black text-white tracking-tight uppercase">System Export</h3>
                <p className="text-slate-500 text-sm font-medium leading-relaxed">
                  Generate a complete encrypted SQL snapshot of the entire data cluster across all entities.
                </p>
                <button onClick={handleExport} className="btn btn-secondary w-full py-5 text-base font-black uppercase tracking-[0.2em]">
                  Generate Backup
                </button>
              </div>

              <div className="card-glass border-white/10 p-10 space-y-6">
                <div className="w-16 h-16 rounded-2xl bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400 mb-2">
                  <Upload className="w-8 h-8" />
                </div>
                <h3 className="text-2xl font-black text-white tracking-tight uppercase">Data Ingestion</h3>
                <p className="text-slate-500 text-sm font-medium leading-relaxed">
                  Execute direct protocol streams into the cluster to restore or synchronize datasets.
                </p>
                <textarea
                  value={importSQL}
                  onChange={(e) => setImportSQL(e.target.value)}
                  placeholder="-- PASTE SQL DUMP CONTENT..."
                  className="w-full h-40 bg-black/40 border border-white/10 rounded-2xl p-6 text-emerald-400 font-mono text-xs focus:outline-none focus:border-indigo-500 mb-2 resize-none"
                />
                <button 
                  onClick={handleImport}
                  disabled={importing || !importSQL.trim()}
                  className="btn btn-primary w-full py-5 text-base font-black uppercase tracking-[0.2em] disabled:opacity-50"
                >
                  {importing ? <RefreshCw className="w-6 h-6 animate-spin" /> : 'Run Sync Operation'}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Credentials Modal */}
      {showCredentials && credentials && (
        <div className="fixed inset-0 z-[110] flex items-center justify-center p-4">
          <div className="fixed inset-0 bg-black/80 backdrop-blur-md" onClick={() => setShowCredentials(false)} />
          <div className="relative w-full max-w-md card-glass border-white/10 p-10 animate-pop-in">
            <div className="flex justify-between items-center mb-10">
              <div className="flex items-center gap-4">
                <div className="w-12 h-12 rounded-2xl bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
                  <Key className="w-6 h-6" />
                </div>
                <h3 className="text-2xl font-black text-white tracking-tight uppercase">Cluster Key</h3>
              </div>
              <button onClick={() => setShowCredentials(false)} className="text-slate-500 hover:text-white transition-all"><X className="w-6 h-6" /></button>
            </div>
            
            <div className="space-y-6">
              {[
                { label: 'Host Access', value: credentials.host },
                { label: 'Ingress Port', value: credentials.port },
                { label: 'Target DB', value: credentials.database },
                { label: 'Root Sync User', value: credentials.username },
                { label: 'Access Token', value: credentials.password, secret: true },
              ].map(item => (
                <div key={item.label} className="group space-y-2">
                  <span className="text-[10px] font-black text-slate-500 uppercase tracking-widest ml-1">{item.label}</span>
                  <div className="flex items-center justify-between bg-white/[0.03] rounded-2xl p-4 border border-white/5 group-hover:border-indigo-500/30 transition-all">
                    <code className="text-indigo-400 font-mono text-xs truncate max-w-[200px]">
                      {item.value}
                    </code>
                    <button 
                      onClick={() => copyToClipboard(item.value)}
                      className="w-10 h-10 flex items-center justify-center rounded-xl bg-white/5 text-slate-600 hover:text-white transition-all"
                    >
                      <Copy className="w-4 h-4" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
            
            <button 
              onClick={() => setShowCredentials(false)}
              className="btn btn-secondary w-full py-5 text-sm font-black uppercase tracking-[0.2em] mt-10"
            >
              Close Secure View
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

