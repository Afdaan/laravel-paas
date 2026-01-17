// ===========================================
// Database Manager Component
// ===========================================
// User-friendly UI for students to manage their database
// ===========================================

import { useState, useEffect } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'
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
  const [tableStructure, setTableStructure] = useState(null)
  const [loading, setLoading] = useState(true)
  const [credentials, setCredentials] = useState(null)
  
  // Query state
  const [query, setQuery] = useState('')
  const [queryResult, setQueryResult] = useState(null)
  const [queryLoading, setQueryLoading] = useState(false)
  
  // Import state
  const [importSQL, setImportSQL] = useState('')
  const [importing, setImporting] = useState(false)
  
  // Modals
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
      toast.error('Failed to load project')
      if (err.response?.status === 404) navigate('/projects')
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
      toast.error('Failed to fetch tables')
    } finally {
      setLoading(false)
    }
  }

  const selectTable = async (tableName) => {
    setSelectedTable(tableName)
    setLoading(true)
    setTableData(null)
    try {
      const [structureRes, dataRes] = await Promise.all([
        databaseAPI.getStructure(id, tableName),
        databaseAPI.getData(id, tableName, 1, 50)
      ])
      setTableStructure(structureRes.data.columns || [])
      setTableData(dataRes.data)
    } catch (err) {
      toast.error('Failed to load table data')
    } finally {
      setLoading(false)
    }
  }

  const executeQuery = async () => {
    if (!query.trim()) return
    setQueryLoading(true)
    setQueryResult(null)
    try {
      const res = await databaseAPI.query(id, query)
      setQueryResult(res.data)
      toast.success(`Query executed in ${res.data.duration}`)
    } catch (err) {
      toast.error(err.response?.data?.error || 'Query failed')
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
      a.download = `${project?.database_name}_backup.sql`
      a.click()
      window.URL.revokeObjectURL(url)
      toast.success('Database exported successfully!')
    } catch (err) {
      toast.error('Export failed')
    }
  }

  const handleImport = async () => {
    if (!importSQL.trim()) {
      toast.error('Please paste SQL content')
      return
    }
    setImporting(true)
    try {
      const res = await databaseAPI.import(id, importSQL)
      if (res.data.success) {
        toast.success(`Imported ${res.data.statements} statements`)
        setImportSQL('')
        fetchTables()
      } else {
        toast.error(`Import completed with errors: ${res.data.errors?.length || 0} errors`)
      }
    } catch (err) {
      toast.error('Import failed')
    } finally {
      setImporting(false)
    }
  }

  const confirmReset = () => {
    setConfirmModal({
      isOpen: true,
      title: 'Reset Database?',
      message: 'This will PERMANENTLY DELETE all tables and data in this database. This action cannot be undone.',
      type: 'danger',
      confirmText: 'Yes, Reset Database',
      onConfirm: handleReset
    })
  }

  const handleReset = async () => {
    try {
      const res = await databaseAPI.reset(id)
      toast.success(`Database reset! Dropped ${res.data.dropped} tables`)
      setSelectedTable(null)
      setTableData(null)
      fetchTables()
    } catch (err) {
      toast.error('Reset failed')
    }
  }

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text)
    toast.success('Copied')
  }

  return (
    <div className="space-y-6 max-w-7xl mx-auto pb-20">
      <ConfirmationModal 
        isOpen={confirmModal.isOpen}
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
      />

      {/* Header - Only show if not embedded */}
      {!embedded && (
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-slate-700 pb-6">
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Link to={`/projects/${id}`} className="text-slate-400 hover:text-white transition-colors flex items-center gap-1 text-sm">
                <span className="i-lucide-arrow-left">←</span> Back to Project
              </Link>
            </div>
            <h1 className="text-3xl font-bold text-white tracking-tight flex items-center gap-3">
              <span className="i-lucide-database text-primary-500"></span>
              Database Manager
            </h1>
            <p className="text-slate-400 mt-1 font-mono text-sm">
              {project?.database_name}
            </p>
          </div>
          <div className="flex gap-3">
            <button 
              onClick={() => setShowCredentials(true)}
              className="btn btn-secondary flex items-center gap-2"
            >
              <span className="i-lucide-key"></span> Credentials
            </button>
            <button 
              onClick={confirmReset}
              className="btn btn-danger flex items-center gap-2"
            >
              <span className="i-lucide-trash"></span> Reset DB
            </button>
          </div>
        </div>
      )}

      {/* Tabs */}
      <div>
         <div className="flex gap-1 bg-slate-800/50 p-1 rounded-lg w-fit mb-6">
           {[
             { id: 'tables', label: 'Tables' },
             { id: 'query', label: 'SQL Query' },
             { id: 'import', label: 'Import / Export' },
           ].map(tab => (
             <button
               key={tab.id}
               onClick={() => setActiveTab(tab.id)}
               className={`px-6 py-2 rounded-md text-sm font-medium transition-all ${
                 activeTab === tab.id 
                 ? 'bg-slate-700 text-white shadow-sm' 
                 : 'text-slate-400 hover:text-slate-200'
               }`}
             >
               {tab.label}
             </button>
           ))}
        </div>

        {/* Tables Tab */}
        {activeTab === 'tables' && (
          <div className="grid grid-cols-1 lg:grid-cols-4 gap-6 h-[700px]">
            {/* Table List Sidebar */}
            <div className="lg:col-span-1 flex flex-col h-full card p-0 overflow-hidden bg-slate-900 border-slate-800">
              <div className="p-4 border-b border-slate-800 bg-slate-800/50 flex justify-between items-center">
                <h3 className="font-semibold text-white">Tables</h3>
                <button onClick={fetchTables} className="text-slate-400 hover:text-white" title="Refresh">
                  <span className="i-lucide-refresh-cw">↻</span>
                </button>
              </div>
              <div className="flex-1 overflow-y-auto p-2 space-y-1">
                {tables.length === 0 && !loading ? (
                   <div className="text-center py-10 text-slate-500 text-sm">No tables found</div>
                ) : (
                  tables.map(table => (
                    <button
                      key={table.name}
                      onClick={() => selectTable(table.name)}
                      className={`w-full text-left px-3 py-2.5 rounded-md transition-all flex justify-between items-baseline group ${
                        selectedTable === table.name 
                          ? 'bg-primary-600 text-white shadow-md' 
                          : 'text-slate-300 hover:bg-slate-800'
                      }`}
                    >
                      <span className="font-medium truncate">{table.name}</span>
                      <span className={`text-xs ${selectedTable === table.name ? 'text-primary-200' : 'text-slate-500 group-hover:text-slate-400'}`}>
                        {table.rows}
                      </span>
                    </button>
                  ))
                )}
              </div>
            </div>

            {/* Table Data View */}
            <div className="lg:col-span-3 flex flex-col h-full card p-0 overflow-hidden border-slate-800">
              {selectedTable ? (
                <>
                  <div className="p-4 border-b border-slate-800 bg-slate-800/30 flex justify-between items-center">
                    <div className="flex items-center gap-3">
                      <h3 className="font-bold text-white text-lg">{selectedTable}</h3>
                      {tableData && (
                        <span className="text-xs px-2 py-0.5 bg-slate-700 rounded text-slate-300">
                          {tableData.total} rows
                        </span>
                      )}
                    </div>
                    {/* Could add structure toggle here if needed */}
                  </div>
                  
                  <div className="flex-1 overflow-auto bg-slate-900/50">
                    {loading ? (
                      <div className="flex items-center justify-center h-full text-slate-500">Loading data...</div>
                    ) : tableData && tableData.rows?.length > 0 ? (
                      <table className="w-full text-left text-sm border-collapse">
                        <thead className="bg-slate-800 sticky top-0 z-10 text-slate-300">
                          <tr>
                            {tableData.columns?.map(col => (
                              <th key={col} className="px-4 py-3 font-mono text-xs uppercase tracking-wider border-b border-slate-700 whitespace-nowrap">
                                {col}
                              </th>
                            ))}
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-slate-800 text-slate-300">
                          {tableData.rows.map((row, i) => (
                            <tr key={i} className="hover:bg-slate-800/50 transition-colors">
                              {tableData.columns?.map(col => (
                                <td key={col} className="px-4 py-2 max-w-xs truncate border-r border-slate-800/50 last:border-0 font-mono text-xs">
                                  {row[col] === null ? (
                                    <span className="text-slate-600 italic">NULL</span>
                                  ) : (
                                    String(row[col])
                                  )}
                                </td>
                              ))}
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    ) : (
                      <div className="flex flex-col items-center justify-center h-full text-slate-500 gap-2">
                        <div className="text-4xl opacity-20">📭</div>
                        <div>Table is empty</div>
                      </div>
                    )}
                  </div>
                </>
              ) : (
                <div className="flex flex-col items-center justify-center h-full text-slate-500 gap-4">
                  <div className="w-16 h-16 rounded-full bg-slate-800 flex items-center justify-center text-3xl opacity-50">👈</div>
                  <p>Select a table from the sidebar to view data</p>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Query Tab */}
        {activeTab === 'query' && (
          <div className="h-[700px] flex flex-col gap-6">
            <div className="card p-0 overflow-hidden flex flex-col h-1/2 border-slate-800">
              <div className="p-3 bg-slate-800 border-b border-slate-700 flex justify-between items-center">
                 <h3 className="font-semibold text-white text-sm">SQL Editor</h3>
                 <div className="flex gap-2">
                    <button onClick={() => setQuery('')} className="text-xs text-slate-400 hover:text-white px-2 py-1 rounded hover:bg-slate-700">Clear</button>
                    <button 
                      onClick={executeQuery}
                      disabled={queryLoading || !query.trim()}
                      className="bg-primary-600 hover:bg-primary-500 text-white text-xs px-3 py-1 rounded font-medium disabled:opacity-50 transition-colors flex items-center gap-1"
                    >
                      {queryLoading ? 'Executing...' : '▶ Run Query'}
                    </button>
                 </div>
              </div>
              <textarea
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="SELECT * FROM users;"
                className="flex-1 bg-black text-emerald-300 font-mono text-sm p-4 focus:outline-none resize-none"
                spellCheck="false"
              />
            </div>

            <div className="card p-0 overflow-hidden flex flex-col h-1/2 border-slate-800">
               <div className="p-3 bg-slate-800 border-b border-slate-700">
                 <h3 className="font-semibold text-white text-sm">Results</h3>
               </div>
               <div className="flex-1 overflow-auto bg-slate-900/50 p-4">
                  {queryResult ? (
                    <div>
                      <div className="mb-4 flex items-center gap-2 text-xs">
                        {queryResult.rows ? (
                          <span className="px-2 py-0.5 bg-emerald-500/10 text-emerald-400 rounded border border-emerald-500/20">
                            {queryResult.rows.length} rows retrieved
                          </span>
                        ) : (
                           <span className="px-2 py-0.5 bg-blue-500/10 text-blue-400 rounded border border-blue-500/20">
                            {queryResult.rows_affected} rows affected
                          </span>
                        )}
                        <span className="text-slate-500">in {queryResult.duration}</span>
                      </div>

                      {queryResult.rows && queryResult.rows.length > 0 && (
                        <div className="overflow-x-auto rounded border border-slate-700">
                           <table className="w-full text-left text-sm border-collapse">
                            <thead className="bg-slate-800 text-slate-300">
                              <tr>
                                {queryResult.columns?.map(col => (
                                  <th key={col} className="px-4 py-2 font-mono text-xs border-b border-slate-600 whitespace-nowrap">
                                    {col}
                                  </th>
                                ))}
                              </tr>
                            </thead>
                            <tbody className="divide-y divide-slate-700 text-slate-300 bg-slate-900">
                              {queryResult.rows.map((row, i) => (
                                <tr key={i} className="hover:bg-slate-800/50">
                                  {queryResult.columns?.map(col => (
                                    <td key={col} className="px-4 py-2 max-w-xs truncate font-mono text-xs border-r border-slate-800 last:border-0">
                                      {row[col] !== null ? String(row[col]) : <span className="text-slate-600">NULL</span>}
                                    </td>
                                  ))}
                                </tr>
                              ))}
                            </tbody>
                           </table>
                        </div>
                      )}
                    </div>
                  ) : (
                    <div className="h-full flex items-center justify-center text-slate-600 text-sm italic">
                      Execute a query to see results
                    </div>
                  )}
               </div>
            </div>
          </div>
        )}

        {/* Import/Export Tab */}
        {activeTab === 'import' && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div className="card border-slate-800 p-6">
               <h3 className="font-bold text-white text-lg mb-2">📤 Export Database</h3>
               <p className="text-slate-400 text-sm mb-6">
                 Download a complete SQL dump of your database structure and data.
               </p>
               <button onClick={handleExport} className="btn btn-secondary w-full justify-center py-3">
                 <span className="i-lucide-download mr-2"></span> Download SQL Backup
               </button>
            </div>

            <div className="card border-slate-800 p-6">
              <h3 className="font-bold text-white text-lg mb-2">📥 Import SQL</h3>
              <p className="text-slate-400 text-sm mb-4">
                Execute SQL commands to restore a backup or initialize table structures.
              </p>
              <textarea
                 value={importSQL}
                 onChange={(e) => setImportSQL(e.target.value)}
                 placeholder="Paste your SQL commands here..."
                 className="w-full h-40 bg-slate-950 border border-slate-700 rounded-lg p-3 text-slate-300 font-mono text-xs focus:outline-none focus:border-primary-500 mb-4 resize-none"
               />
               <button 
                 onClick={handleImport}
                 disabled={importing || !importSQL.trim()}
                 className="btn btn-primary w-full justify-center py-2"
               >
                 {importing ? 'Importing...' : 'Run Import'}
               </button>
            </div>
          </div>
        )}
      </div>

      {/* Credentials Modal */}
      {showCredentials && credentials && (
        <div className="fixed inset-0 bg-black/80 backdrop-blur-sm flex items-center justify-center z-50 animate-fade-in">
          <div className="card w-full max-w-md bg-slate-900 border-slate-700 shadow-2xl">
            <div className="flex justify-between items-center mb-6">
              <h3 className="font-bold text-white text-lg">🔑 Database Credentials</h3>
              <button onClick={() => setShowCredentials(false)} className="text-slate-400 hover:text-white">✕</button>
            </div>
            
            <div className="space-y-3">
              {[
                { label: 'Host', value: credentials.host },
                { label: 'Port', value: credentials.port },
                { label: 'Database', value: credentials.database },
                { label: 'Username', value: credentials.username },
                { label: 'Password', value: credentials.password },
              ].map(item => (
                <div key={item.label} className="group flex justify-between items-center bg-slate-800 rounded-lg p-3 border border-slate-700">
                  <span className="text-slate-400 text-sm font-medium">{item.label}</span>
                  <div className="flex items-center gap-2">
                    <code className="text-primary-400 font-mono text-sm">{item.value}</code>
                    <button 
                      onClick={() => copyToClipboard(item.value)}
                      className="text-slate-600 hover:text-white opacity-0 group-hover:opacity-100 transition-opacity"
                      title="Copy"
                    >
                      📋
                    </button>
                  </div>
                </div>
              ))}
            </div>
            
            <button 
              onClick={() => setShowCredentials(false)}
              className="btn btn-secondary w-full mt-6"
            >
              Close
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
