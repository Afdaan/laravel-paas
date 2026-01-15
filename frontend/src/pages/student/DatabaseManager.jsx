// ===========================================
// Database Manager Component
// ===========================================
// User-friendly UI for students to manage their database
// ===========================================

import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import { databaseAPI, projectsAPI } from '../../services/api'

export default function DatabaseManager() {
  const { id } = useParams()
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
  const [showResetConfirm, setShowResetConfirm] = useState(false)
  
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
      toast.success('Database exported!')
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

  const handleReset = async () => {
    try {
      const res = await databaseAPI.reset(id)
      toast.success(`Database reset! Dropped ${res.data.dropped} tables`)
      setShowResetConfirm(false)
      setSelectedTable(null)
      setTableData(null)
      fetchTables()
    } catch (err) {
      toast.error('Reset failed')
    }
  }

  const tabs = [
    { id: 'tables', label: '📋 Tables', icon: '📋' },
    { id: 'query', label: '⚡ Query', icon: '⚡' },
    { id: 'import', label: '📥 Import/Export', icon: '📥' },
  ]

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-3 mb-2">
            <Link to={`/projects/${id}`} className="text-gray-400 hover:text-white">
              ← Back to Project
            </Link>
          </div>
          <h1 className="text-2xl font-bold text-white">
            🗄️ Database Manager
          </h1>
          <p className="text-gray-400 mt-1">
            {project?.name} • {project?.database_name}
          </p>
        </div>
        <div className="flex gap-3">
          <button 
            onClick={() => setShowCredentials(true)}
            className="btn-secondary"
          >
            🔑 Credentials
          </button>
          <button 
            onClick={() => setShowResetConfirm(true)}
            className="btn-danger"
          >
            🗑️ Reset DB
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-2 border-b border-gray-700 pb-4">
        {tabs.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`px-4 py-2 rounded-lg font-medium transition-all ${
              activeTab === tab.id 
                ? 'bg-primary-600 text-white' 
                : 'bg-gray-800 text-gray-400 hover:bg-gray-700'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Tables Tab */}
      {activeTab === 'tables' && (
        <div className="grid grid-cols-12 gap-6">
          {/* Table List */}
          <div className="col-span-3">
            <div className="card">
              <h3 className="font-semibold text-white mb-4">Tables</h3>
              {loading && !tables.length ? (
                <div className="text-gray-400 text-center py-4">Loading...</div>
              ) : tables.length === 0 ? (
                <div className="text-gray-400 text-center py-4">
                  No tables found
                </div>
              ) : (
                <div className="space-y-2">
                  {tables.map(table => (
                    <button
                      key={table.name}
                      onClick={() => selectTable(table.name)}
                      className={`w-full text-left px-3 py-2 rounded-lg transition-all ${
                        selectedTable === table.name 
                          ? 'bg-primary-600 text-white' 
                          : 'bg-gray-700/50 text-gray-300 hover:bg-gray-700'
                      }`}
                    >
                      <div className="font-medium">{table.name}</div>
                      <div className="text-xs opacity-70">
                        {table.rows} rows • {table.size}
                      </div>
                    </button>
                  ))}
                </div>
              )}
              <button 
                onClick={fetchTables}
                className="w-full mt-4 btn-secondary text-sm"
              >
                🔄 Refresh
              </button>
            </div>
          </div>

          {/* Table Data */}
          <div className="col-span-9">
            {selectedTable ? (
              <div className="card">
                <div className="flex items-center justify-between mb-4">
                  <h3 className="font-semibold text-white text-lg">
                    📊 {selectedTable}
                  </h3>
                  {tableData && (
                    <span className="text-gray-400 text-sm">
                      {tableData.total} rows total
                    </span>
                  )}
                </div>

                {/* Structure Toggle */}
                {tableStructure && (
                  <details className="mb-4">
                    <summary className="text-primary-400 cursor-pointer hover:text-primary-300">
                      View Structure ({tableStructure.length} columns)
                    </summary>
                    <div className="mt-2 overflow-x-auto">
                      <table className="table text-sm">
                        <thead>
                          <tr>
                            <th>Column</th>
                            <th>Type</th>
                            <th>Nullable</th>
                            <th>Key</th>
                            <th>Extra</th>
                          </tr>
                        </thead>
                        <tbody>
                          {tableStructure.map(col => (
                            <tr key={col.name}>
                              <td className="font-mono text-primary-400">{col.name}</td>
                              <td className="text-gray-400">{col.type}</td>
                              <td>{col.nullable ? '✓' : '✗'}</td>
                              <td className="text-yellow-400">{col.key}</td>
                              <td className="text-gray-500">{col.extra}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </details>
                )}

                {/* Data Table */}
                {loading ? (
                  <div className="text-center py-8 text-gray-400">Loading data...</div>
                ) : tableData && tableData.rows?.length > 0 ? (
                  <div className="overflow-x-auto max-h-96">
                    <table className="table text-sm">
                      <thead className="sticky top-0 bg-gray-800">
                        <tr>
                          {tableData.columns?.map(col => (
                            <th key={col} className="font-mono">{col}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        {tableData.rows.map((row, i) => (
                          <tr key={i}>
                            {tableData.columns?.map(col => (
                              <td key={col} className="truncate max-w-xs">
                                {row[col] !== null && row[col] !== undefined 
                                  ? String(row[col]).substring(0, 100) 
                                  : <span className="text-gray-500">NULL</span>}
                              </td>
                            ))}
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <div className="text-center py-8 text-gray-400">
                    No data in this table
                  </div>
                )}
              </div>
            ) : (
              <div className="card text-center py-16">
                <div className="text-6xl mb-4">👈</div>
                <p className="text-gray-400">Select a table to view its data</p>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Query Tab */}
      {activeTab === 'query' && (
        <div className="card">
          <h3 className="font-semibold text-white mb-4">⚡ Run SQL Query</h3>
          <textarea
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="SELECT * FROM users LIMIT 10;"
            className="input w-full h-32 font-mono text-sm"
            spellCheck="false"
          />
          <div className="flex gap-3 mt-4">
            <button 
              onClick={executeQuery}
              disabled={queryLoading || !query.trim()}
              className="btn-primary"
            >
              {queryLoading ? '⏳ Executing...' : '▶️ Run Query'}
            </button>
            <button 
              onClick={() => setQuery('')}
              className="btn-secondary"
            >
              Clear
            </button>
          </div>

          {/* Query Result */}
          {queryResult && (
            <div className="mt-6">
              <div className="flex items-center gap-4 mb-3">
                <h4 className="font-medium text-white">Results</h4>
                <span className="text-sm text-gray-400">
                  {queryResult.rows ? `${queryResult.rows.length} rows` : `${queryResult.rows_affected} rows affected`}
                  {' • '}{queryResult.duration}
                </span>
              </div>
              
              {queryResult.rows && queryResult.rows.length > 0 ? (
                <div className="overflow-x-auto max-h-96">
                  <table className="table text-sm">
                    <thead className="sticky top-0 bg-gray-800">
                      <tr>
                        {queryResult.columns?.map(col => (
                          <th key={col} className="font-mono">{col}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {queryResult.rows.map((row, i) => (
                        <tr key={i}>
                          {queryResult.columns?.map(col => (
                            <td key={col} className="truncate max-w-xs">
                              {row[col] !== null ? String(row[col]) : <span className="text-gray-500">NULL</span>}
                            </td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : queryResult.rows_affected !== undefined ? (
                <div className="bg-green-900/30 border border-green-700 rounded-lg p-4 text-green-400">
                  ✓ Query executed successfully. {queryResult.rows_affected} rows affected.
                </div>
              ) : (
                <div className="text-gray-400">No results</div>
              )}
            </div>
          )}
        </div>
      )}

      {/* Import/Export Tab */}
      {activeTab === 'import' && (
        <div className="grid grid-cols-2 gap-6">
          {/* Export */}
          <div className="card">
            <h3 className="font-semibold text-white mb-4">📤 Export Database</h3>
            <p className="text-gray-400 mb-4">
              Download a backup of your database as a SQL file.
            </p>
            <button onClick={handleExport} className="btn-primary w-full">
              📥 Download SQL Backup
            </button>
          </div>

          {/* Import */}
          <div className="card">
            <h3 className="font-semibold text-white mb-4">📥 Import SQL</h3>
            <p className="text-gray-400 mb-4">
              Paste SQL content to import into your database.
            </p>
            <textarea
              value={importSQL}
              onChange={(e) => setImportSQL(e.target.value)}
              placeholder="-- Paste your SQL here...&#10;CREATE TABLE...&#10;INSERT INTO..."
              className="input w-full h-40 font-mono text-sm mb-4"
              spellCheck="false"
            />
            <button 
              onClick={handleImport}
              disabled={importing || !importSQL.trim()}
              className="btn-primary w-full"
            >
              {importing ? '⏳ Importing...' : '📥 Import SQL'}
            </button>
          </div>
        </div>
      )}

      {/* Credentials Modal */}
      {showCredentials && credentials && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="card w-full max-w-md">
            <h3 className="font-semibold text-white text-lg mb-4">🔑 Database Credentials</h3>
            <div className="space-y-3">
              {[
                { label: 'Host', value: credentials.host },
                { label: 'Port', value: credentials.port },
                { label: 'Database', value: credentials.database },
                { label: 'Username', value: credentials.username },
                { label: 'Password', value: credentials.password },
              ].map(item => (
                <div key={item.label} className="flex justify-between items-center bg-gray-700/50 rounded-lg p-3">
                  <span className="text-gray-400">{item.label}</span>
                  <code className="text-primary-400 font-mono">{item.value}</code>
                </div>
              ))}
            </div>
            <button 
              onClick={() => setShowCredentials(false)}
              className="btn-secondary w-full mt-4"
            >
              Close
            </button>
          </div>
        </div>
      )}

      {/* Reset Confirmation Modal */}
      {showResetConfirm && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="card w-full max-w-md">
            <h3 className="font-semibold text-red-400 text-lg mb-4">⚠️ Reset Database</h3>
            <p className="text-gray-300 mb-4">
              This will <strong className="text-red-400">permanently delete all tables and data</strong> in your database. 
              This action cannot be undone!
            </p>
            <div className="flex gap-3">
              <button 
                onClick={() => setShowResetConfirm(false)}
                className="btn-secondary flex-1"
              >
                Cancel
              </button>
              <button 
                onClick={handleReset}
                className="btn-danger flex-1"
              >
                Yes, Reset Database
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
