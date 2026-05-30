import React, { useState, useEffect, useCallback } from 'react'
import { useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { databaseAPI } from '../../services/api'
import { DatabaseBackup } from '../../types'
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
  Search,
  Download,
  Pencil,
  MoreHorizontal
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import useTranslation from '@/lib/useTranslation'
import ConfirmationModal from '@/components/ConfirmationModal'

const parseDbType = (dbType: string) => {
  const typeLower = dbType.toLowerCase();
  let length: number | string = 255;
  const match = typeLower.match(/\((\d+)\)/);
  if (match) {
    length = parseInt(match[1], 10);
  }
  
  if (typeLower.includes('varchar') || typeLower.includes('string') || typeLower.includes('char')) {
    return { type: 'varchar', length };
  }
  if (typeLower.includes('tinyint(1)') || typeLower.includes('bool') || typeLower.includes('boolean')) {
    return { type: 'boolean', length: '' };
  }
  if (typeLower.includes('bigint')) {
    return { type: 'bigint', length: '' };
  }
  if (typeLower.includes('int') || typeLower.includes('integer')) {
    return { type: 'integer', length: '' };
  }
  if (typeLower.includes('text')) {
    return { type: 'text', length: '' };
  }
  if (typeLower.includes('timestamp') || typeLower.includes('datetime') || typeLower.includes('date')) {
    return { type: 'timestamp', length: '' };
  }
  if (typeLower.includes('decimal')) {
    return { type: 'decimal', length: '' };
  }
  return { type: 'varchar', length: 255 };
};

const toLocalISOString = (d: Date): string => {
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const hours = String(d.getHours()).padStart(2, '0')
  const minutes = String(d.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day}T${hours}:${minutes}`
}

const toLocalDateString = (d: Date): string => {
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const formatDatetimeLocal = (val: any) => {
  if (val === null || val === undefined) return ''
  try {
    if (val instanceof Date) {
      if (isNaN(val.getTime())) return ''
      return toLocalISOString(val)
    }

    const strVal = String(val).trim()
    if (!strVal) return ''

    // If it's already in the exact format YYYY-MM-DDTHH:mm, return it
    if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(strVal)) {
      return strVal
    }

    // Try parsing Unix timestamp if it's numeric
    if (/^\d+$/.test(strVal)) {
      const num = Number(strVal)
      const date = new Date(strVal.length === 10 ? num * 1000 : num)
      if (!isNaN(date.getTime())) {
        return toLocalISOString(date)
      }
    }

    // Direct regex parsing for common DB datetime formats (with optional ms/us, but no timezone)
    // to preserve exact local database representation without browser timezone shifting.
    const regex = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(Z|[+-]\d{2}(?::?\d{2})?)?$/i
    const match = strVal.match(regex)
    if (match) {
      const [_, year, month, day, hours, minutes, , , tz] = match
      if (!tz) {
        return `${year}-${month}-${day}T${hours}:${minutes}`
      }
    }

    // Fallback: standard Date parsing
    const parsedStr = strVal.includes(' ') && !strVal.includes('T') ? strVal.replace(' ', 'T') : strVal
    const d = new Date(parsedStr)
    if (isNaN(d.getTime())) return ''
    return toLocalISOString(d)
  } catch (e) {
    return ''
  }
}

const formatDate = (val: any) => {
  if (val === null || val === undefined) return ''
  try {
    if (val instanceof Date) {
      if (isNaN(val.getTime())) return ''
      return toLocalDateString(val)
    }

    const strVal = String(val).trim()
    if (!strVal) return ''

    // If it's already in the exact format YYYY-MM-DD, return it
    if (/^\d{4}-\d{2}-\d{2}$/.test(strVal)) {
      return strVal
    }

    // Try parsing Unix timestamp if it's numeric
    if (/^\d+$/.test(strVal)) {
      const num = Number(strVal)
      const date = new Date(strVal.length === 10 ? num * 1000 : num)
      if (!isNaN(date.getTime())) {
        return toLocalDateString(date)
      }
    }

    // Try matching YYYY-MM-DD from the beginning
    const regex = /^(\d{4})-(\d{2})-(\d{2})/i
    const match = strVal.match(regex)
    if (match) {
      const [_, year, month, day] = match
      return `${year}-${month}-${day}`
    }

    // Fallback: standard Date parsing
    const parsedStr = strVal.includes(' ') && !strVal.includes('T') ? strVal.replace(' ', 'T') : strVal
    const d = new Date(parsedStr)
    if (isNaN(d.getTime())) return ''
    return toLocalDateString(d)
  } catch (e) {
    return ''
  }
}

interface DatabaseStudioProps {
  projectId?: string | number | null;
  embedded?: boolean;
}

function DatabaseStudio({ projectId = null, embedded = false }: DatabaseStudioProps) {
  const params = useParams<{ id: string }>()
  const id = projectId || params.id
  const { t } = useTranslation()
  
  const [activeTab, setActiveTab] = useState<'dashboard' | 'tables' | 'structure' | 'query' | 'backups'>('dashboard')
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
  const [queryHistory, setQueryHistory] = useState<string[]>(() => {
    if (typeof window !== 'undefined' && id) {
      try {
        const saved = localStorage.getItem(`db_query_history_${id}`)
        return saved ? JSON.parse(saved) : []
      } catch (e) {
        return []
      }
    }
    return []
  })
  
  // Table Viewer states
  const [selectedTable, setSelectedTable] = useState<string>('')
  const [tableData, setTableData] = useState<any>(null)
  const [tablePage, setTablePage] = useState(1)
  const [tableLimit] = useState(25)
  const [tableTotal, setTableTotal] = useState(0)
  const [tableSearch, setTableSearch] = useState('')
  const [structureSearch, setStructureSearch] = useState('')

  // Dynamic visual insert row modal states
  const [showInsertModal, setShowInsertModal] = useState(false)
  const [insertFormData, setInsertFormData] = useState<Record<string, any>>({})

  // Edit row states
  const [showEditModal, setShowEditModal] = useState(false)
  const [editingRow, setEditingRow] = useState<any>(null)
  const [editFormData, setEditFormData] = useState<Record<string, any>>({})
  const [showRowEditPreview, setShowRowEditPreview] = useState(false)
  const [rowEditPreviewSql, setRowEditPreviewSql] = useState('')

  // Modify column states
  const [editingCol, setEditingCol] = useState<any>(null)
  const [editColNewName, setEditColNewName] = useState('')
  const [editColType, setEditColType] = useState('varchar')
  const [editColLength, setEditColLength] = useState<number | string>(255)
  const [editColNullable, setEditColNullable] = useState(true)
  const [editColDefault, setEditColDefault] = useState('')
  const [showColModifyPreview, setShowColModifyPreview] = useState(false)
  const [colModifyPreviewSql, setColModifyPreviewSql] = useState('')

  // Confirmation Modal state
  const [confirmModal, setConfirmModal] = useState<{
    isOpen: boolean;
    title: string;
    message: string;
    type: 'danger' | 'warning' | 'info';
    confirmText?: string;
    cancelText?: string;
    onConfirm: () => void;
  }>({
    isOpen: false,
    title: '',
    message: '',
    type: 'danger',
    onConfirm: () => {}
  })

  const triggerConfirmation = (options: {
    title: string;
    message: string;
    type: 'danger' | 'warning' | 'info';
    confirmText?: string;
    cancelText?: string;
    onConfirm: () => void;
  }) => {
    setConfirmModal({
      isOpen: true,
      ...options
    })
  }

  const closeConfirmation = () => {
    setConfirmModal(prev => ({ ...prev, isOpen: false }))
  }

  const filteredTables = schemaData.filter(t => 
    t.name.toLowerCase().includes(tableSearch.toLowerCase())
  )
  
  // Visual Designer states
  const [designerAction, setDesignerAction] = useState<'create_table' | 'add_column' | 'create_index' | 'modify_column' | null>(null)
  const [newTableName, setNewTableName] = useState('')
  const [newColName, setNewColName] = useState('')
  const [newColType, setNewColType] = useState('varchar')
  const [newColLength, setNewColLength] = useState<number | string>(255)
  const [newColNullable, setNewColNullable] = useState(true)
  
  const [indexName] = useState('')
  const [indexCols] = useState<string[]>([])

  // Load complete studio dataset
  const loadStudioData = useCallback(async () => {
    if (!id) return
    setIsLoading(true)
    try {
      const overviewRes = await databaseAPI.getOverview(id)
      setDbOverview(overviewRes.data)
      
      const schemaRes = await databaseAPI.getSchema(id)
      const tables = schemaRes.data.tables || []
      setSchemaData(tables)
      
      setSelectedTable(current => {
        const exists = tables.some((t: any) => t.name === current)
        if (exists && current) return current
        return tables.length > 0 ? tables[0].name : ''
      })
      
      const backupsRes = await databaseAPI.listBackups(id)
      setBackups(backupsRes.data.backups || [])
      
      const metricsRes = await databaseAPI.getMetrics(id)
      setMetrics(metricsRes.data)
    } catch (err: any) {
      toast.error(t('databaseStudio.errors.connectFailed'))
    } finally {
      setIsLoading(false)
    }
  }, [id, t])

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
      toast.error(t('databaseStudio.errors.readRowsFailed'))
    }
  }, [id, selectedTable, tablePage, tableLimit, t])

  useEffect(() => {
    if (activeTab === 'tables' && selectedTable) {
      loadTableDataGrid()
    }
  }, [activeTab, selectedTable, tablePage, loadTableDataGrid])

  // Load query history on database ID change
  useEffect(() => {
    if (!id) return
    try {
      const saved = localStorage.getItem(`db_query_history_${id}`)
      setQueryHistory(saved ? JSON.parse(saved) : [])
    } catch (e) {
      setQueryHistory([])
    }
  }, [id])

  // Action helpers
  const handleRotateCredentials = () => {
    if (!id) return
    triggerConfirmation({
      title: t('databaseStudio.dashboard.actions.rotateCredentials'),
      message: t('databaseStudio.dashboard.confirmRotate'),
      type: 'danger',
      confirmText: t('databaseStudio.dashboard.actions.rotateCredentials'),
      onConfirm: async () => {
        setIsActionLoading(true)
        try {
          const res = await databaseAPI.rotateCredentials(id)
          toast.success(res.data.message)
          loadStudioData()
        } catch (err: any) {
          toast.error(t('databaseStudio.errors.rotateFailed'))
        } finally {
          setIsActionLoading(false)
        }
      }
    })
  }

  const handleRestartPool = async () => {
    if (!id) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.restartDatabase(id)
      toast.success(res.data.message)
    } catch (err: any) {
      toast.error(t('databaseStudio.errors.testConnectionFailed') + ': ' + (err.response?.data?.error || err.message))
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleToggleStatus = (suspend: boolean) => {
    if (!id) return
    triggerConfirmation({
      title: suspend ? t('databaseStudio.dashboard.actions.suspendDatabase') : t('databaseStudio.dashboard.actions.resumeDatabase'),
      message: suspend ? t('databaseStudio.dashboard.confirmSuspend') : t('databaseStudio.dashboard.confirmResume'),
      type: suspend ? 'danger' : 'info',
      confirmText: suspend ? t('databaseStudio.dashboard.actions.suspendDatabase') : t('databaseStudio.dashboard.actions.resumeDatabase'),
      onConfirm: async () => {
        setIsActionLoading(true)
        try {
          const res = await databaseAPI.updateStatus(id, suspend)
          toast.success(res.data.message)
          loadStudioData()
        } catch (err: any) {
          toast.error(t('databaseStudio.errors.updateStatusFailed'))
        } finally {
          setIsActionLoading(false)
        }
      }
    })
  }

  const handleExecuteSQL = async () => {
    if (!id || !sqlQuery.trim()) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.query(id, sqlQuery)
      setQueryResult(res.data)
      
      setQueryHistory(prev => {
        const newHistory = [sqlQuery, ...prev.filter(q => q !== sqlQuery).slice(0, 49)]
        try {
          localStorage.setItem(`db_query_history_${id}`, JSON.stringify(newHistory))
        } catch (e) {
          console.error(e)
        }
        return newHistory
      })
      
      toast.success(t('databaseStudio.query.successToast'))
    } catch (err: any) {
      const errMsg = err.response?.data?.error || err.message
      setQueryResult({ error: errMsg })
      toast.error(t('databaseStudio.query.failedToast') + ': ' + errMsg)
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleClearQueryHistory = () => {
    triggerConfirmation({
      title: t('databaseStudio.query.history.clearConfirmTitle'),
      message: t('databaseStudio.query.history.clearConfirmDesc'),
      type: 'danger',
      confirmText: t('databaseStudio.query.history.clearConfirmAction'),
      onConfirm: () => {
        setQueryHistory([])
        try {
          localStorage.removeItem(`db_query_history_${id}`)
        } catch (e) {
          console.error(e)
        }
      }
    })
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
      toast.error(t('databaseStudio.errors.createBackupFailed'))
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleRestoreBackup = (backupId: number) => {
    if (!id) return
    triggerConfirmation({
      title: t('databaseStudio.backups.confirmRestoreTitle'),
      message: t('databaseStudio.backups.confirmRestoreDesc'),
      type: 'danger',
      confirmText: t('databaseStudio.backups.confirmRestoreAction'),
      onConfirm: async () => {
        setIsActionLoading(true)
        try {
          const res = await databaseAPI.restoreBackup(id, backupId)
          toast.success(res.data.message)
          loadStudioData()
        } catch (err: any) {
          toast.error(t('databaseStudio.errors.restoreBackupFailed'))
        } finally {
          setIsActionLoading(false)
        }
      }
    })
  }

  const handleDeleteBackup = (backupId: number) => {
    if (!id) return
    triggerConfirmation({
      title: t('databaseStudio.backups.confirmDeleteTitle'),
      message: t('databaseStudio.backups.confirmDeleteDesc'),
      type: 'danger',
      confirmText: t('databaseStudio.backups.confirmDeleteAction'),
      onConfirm: async () => {
        setIsActionLoading(true)
        try {
          const res = await databaseAPI.deleteBackup(id, backupId)
          toast.success(res.data.message)
          setBackups(prev => prev.filter(b => b.id !== backupId))
        } catch (err: any) {
          toast.error(t('databaseStudio.errors.deleteBackupFailed'))
        } finally {
          setIsActionLoading(false)
        }
      }
    })
  }

  const handleDownloadBackup = async (backupId: number, filename: string) => {
    if (!id) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.downloadBackup(id, backupId)
      const blob = new Blob([res.data], { type: 'application/octet-stream' })
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.setAttribute('download', filename)
      document.body.appendChild(link)
      link.click()
      link.remove()
      window.URL.revokeObjectURL(url)
      toast.success(t('common.success') || 'Download started successfully')
    } catch (err: any) {
      toast.error(t('databaseStudio.errors.downloadBackupFailed'))
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleDropColumn = (tableName: string, colName: string) => {
    if (!id) return
    triggerConfirmation({
      title: t('databaseStudio.structure.actions.dropColumn'),
      message: t('databaseStudio.structure.actions.dropColumnConfirmDesc', { column: colName, table: tableName }),
      type: 'danger',
      confirmText: t('databaseStudio.structure.actions.dropColumn'),
      onConfirm: async () => {
        setIsActionLoading(true)
        try {
          await databaseAPI.executeDesigner(id, {
            action: 'drop_column',
            table_name: tableName,
            index_name: colName // Reuse IndexName for Column Name here as per backend GORM spec
          })
          toast.success(t('databaseStudio.structure.updateSuccess'))
          loadStudioData()
        } catch (err: any) {
          toast.error(err.response?.data?.error || t('databaseStudio.errors.designerActionFailed'))
        } finally {
          setIsActionLoading(false)
        }
      }
    })
  }

  const handleDeleteTable = (tableName: string) => {
    if (!id) return
    triggerConfirmation({
      title: t('databaseStudio.structure.actions.dropTable'),
      message: t('databaseStudio.structure.actions.dropTableConfirmDesc', { table: tableName }),
      type: 'danger',
      confirmText: t('databaseStudio.structure.actions.dropTable'),
      onConfirm: async () => {
        setIsActionLoading(true)
        try {
          await databaseAPI.executeDesigner(id, {
            action: 'drop_table',
            table_name: tableName
          })
          toast.success(t('databaseStudio.structure.updateSuccess'))
          
          if (selectedTable === tableName) {
            setSelectedTable('')
            setTableData(null)
          }

          loadStudioData()
        } catch (err: any) {
          toast.error(err.response?.data?.error || t('databaseStudio.errors.designerActionFailed'))
        } finally {
          setIsActionLoading(false)
        }
      }
    })
  }

  const handleDeleteRow = (row: any, pkCol: string) => {
    if (!id || !selectedTable) return
    const pkValue = row[pkCol]
    if (pkValue == null) {
      toast.error(t('databaseStudio.errors.missingPrimaryKey'))
      return
    }
    triggerConfirmation({
      title: t('databaseStudio.tables.deleteRowConfirmTitle'),
      message: t('databaseStudio.tables.deleteRowConfirmDesc'),
      type: 'danger',
      confirmText: t('databaseStudio.tables.deleteRowAction'),
      onConfirm: async () => {
        setIsActionLoading(true)
        try {
          await databaseAPI.deleteRow(id, selectedTable, pkCol, pkValue)
          toast.success(t('databaseStudio.tables.deleteRowSuccess'))
          loadTableDataGrid()
        } catch (err: any) {
          toast.error(err.response?.data?.error || t('databaseStudio.errors.deleteRowFailed'))
        } finally {
          setIsActionLoading(false)
        }
      }
    })
  }

  const handleInsertRowSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!id || !selectedTable) return

    setIsActionLoading(true)
    try {
      const cols = schemaData.find(t => t.name === selectedTable)?.columns || []
      const fields: string[] = []
      const values: string[] = []

      const isPostgres = dbOverview?.engine?.toLowerCase() === 'postgres'
      const q = isPostgres ? '"' : '`'

      cols.forEach((col: any) => {
        if (col.key === 'PRI' && !insertFormData[col.name]) return

        const val = insertFormData[col.name]
        if (val === undefined || val === '') return

        fields.push(`${q}${col.name}${q}`)

        const typeLower = col.type.toLowerCase()
        if (typeLower.includes('int') || typeLower.includes('decimal') || typeLower.includes('float') || typeLower.includes('double')) {
          values.push(String(Number(val)))
        } else if (typeLower.includes('bool') || typeLower.includes('tinyint(1)')) {
          values.push(val === 'true' || val === true ? '1' : '0')
        } else {
          const escapedVal = String(val).replace(/'/g, "''")
          values.push(`'${escapedVal}'`)
        }
      })

      if (fields.length === 0) {
        toast.error(t('databaseStudio.errors.designerActionFailed'))
        setIsActionLoading(false)
        return
      }

      const sql = `INSERT INTO ${q}${selectedTable}${q} (${fields.join(', ')}) VALUES (${values.join(', ')});`
      const res = await databaseAPI.query(id, sql)
      if (res.data && res.data.error) {
        throw new Error(res.data.error)
      }

      toast.success(t('databaseStudio.tables.insertModal.success'))
      setShowInsertModal(false)
      loadTableDataGrid()
      loadStudioData()
    } catch (err: any) {
      toast.error(t('databaseStudio.tables.insertModal.failed') + ': ' + (err.response?.data?.error || err.message))
    } finally {
      setIsActionLoading(false)
    }
  }

  const generateRowEditSql = () => {
    if (!selectedTable || !editingRow) return ''
    const pkColumn = tableData?.columns.find((c: string) => 
      c.toLowerCase() === 'id' || c.toLowerCase() === 'uid' || c.toLowerCase() === 'uuid'
    ) || tableData?.columns[0]
    const pkValue = editingRow[pkColumn]

    const isPostgres = dbOverview?.engine?.toLowerCase() === 'postgres'
    const q = isPostgres ? '"' : '`'

    const setClauses: string[] = []
    const cols = schemaData.find(t => t.name === selectedTable)?.columns || []

    cols.forEach((col: any) => {
      if (col.name.toLowerCase() === pkColumn.toLowerCase()) return

      const val = editFormData[col.name]
      const escapedCol = `${q}${col.name}${q}`

      if (val === undefined || val === '') {
        if (col.nullable) {
          setClauses.push(`${escapedCol} = NULL`)
        } else {
          const typeLower = col.type.toLowerCase()
          if (typeLower.includes('int') || typeLower.includes('decimal') || typeLower.includes('float') || typeLower.includes('double')) {
            setClauses.push(`${escapedCol} = 0`)
          } else if (typeLower.includes('bool') || typeLower.includes('tinyint(1)')) {
            setClauses.push(`${escapedCol} = false`)
          } else {
            setClauses.push(`${escapedCol} = ''`)
          }
        }
        return
      }

      const typeLower = col.type.toLowerCase()
      if (typeLower.includes('int') || typeLower.includes('decimal') || typeLower.includes('float') || typeLower.includes('double')) {
        setClauses.push(`${escapedCol} = ${Number(val)}`)
      } else if (typeLower.includes('bool') || typeLower.includes('tinyint(1)')) {
        const boolVal = val === 'true' || val === true
        setClauses.push(`${escapedCol} = ${isPostgres ? (boolVal ? 'TRUE' : 'FALSE') : (boolVal ? '1' : '0')}`)
      } else {
        const escapedVal = String(val).replace(/'/g, "''")
        setClauses.push(`${escapedCol} = '${escapedVal}'`)
      }
    })

    const escapedPkValue = typeof pkValue === 'number' ? pkValue : `'${String(pkValue).replace(/'/g, "''")}'`
    return `UPDATE ${q}${selectedTable}${q} SET ${setClauses.join(', ')} WHERE ${q}${pkColumn}${q} = ${escapedPkValue};`
  }

  const openEditRowModal = (row: any) => {
    setEditingRow(row)
    
    const cols = schemaData.find(t => t.name === selectedTable)?.columns || []
    const initialData: Record<string, any> = {}
    cols.forEach((c: any) => {
      // Find row value case-insensitively to prevent schema casing mismatch
      const rowKey = Object.keys(row).find(k => k.toLowerCase() === c.name.toLowerCase()) || c.name
      const val = row[rowKey]
      if (val === null || val === undefined) {
        initialData[c.name] = ''
        return
      }
      
      const typeLower = c.type.toLowerCase()
      if (typeLower.includes('timestamp') || typeLower.includes('datetime')) {
        initialData[c.name] = formatDatetimeLocal(val)
      } else if (typeLower.includes('date')) {
        initialData[c.name] = formatDate(val)
      } else {
        initialData[c.name] = val
      }
    })
    setEditFormData(initialData)
    setShowRowEditPreview(false)
    setRowEditPreviewSql('')
    setShowEditModal(true)
  }

  const handleEditRowFormSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const sql = generateRowEditSql()
    setRowEditPreviewSql(sql)
    setShowRowEditPreview(true)
  }

  const handleEditRowSubmit = async () => {
    if (!id || !selectedTable || !editingRow) return

    const pkColumn = tableData?.columns.find((c: string) => 
      c.toLowerCase() === 'id' || c.toLowerCase() === 'uid' || c.toLowerCase() === 'uuid'
    ) || tableData?.columns[0]

    const pkValue = editingRow[pkColumn]
    if (pkValue == null) {
      toast.error(t('databaseStudio.errors.missingPrimaryKey'))
      return
    }

    setIsActionLoading(true)
    try {
      const cols = schemaData.find(t => t.name === selectedTable)?.columns || []
      const updates: Record<string, any> = {}

      cols.forEach((col: any) => {
        if (col.name.toLowerCase() === pkColumn.toLowerCase()) return

        const val = editFormData[col.name]
        if (val === undefined || val === '') {
          if (col.nullable) {
            updates[col.name] = null
          } else {
            const typeLower = col.type.toLowerCase()
            if (typeLower.includes('int') || typeLower.includes('decimal') || typeLower.includes('float') || typeLower.includes('double')) {
              updates[col.name] = 0
            } else if (typeLower.includes('bool') || typeLower.includes('tinyint(1)')) {
              updates[col.name] = false
            } else {
              updates[col.name] = ""
            }
          }
          return
        }

        const typeLower = col.type.toLowerCase()
        if (typeLower.includes('int') || typeLower.includes('decimal') || typeLower.includes('float') || typeLower.includes('double')) {
          updates[col.name] = Number(val)
        } else if (typeLower.includes('bool') || typeLower.includes('tinyint(1)')) {
          updates[col.name] = val === 'true' || val === true
        } else {
          updates[col.name] = val
        }
      })

      await databaseAPI.updateRow(id, selectedTable, pkColumn, pkValue, updates)
      toast.success(t('databaseStudio.tables.editModal.success'))
      setShowEditModal(false)
      loadTableDataGrid()
      loadStudioData()
    } catch (err: any) {
      toast.error(t('databaseStudio.tables.editModal.failed') + ': ' + (err.response?.data?.error || err.message))
    } finally {
      setIsActionLoading(false)
    }
  }

  const openModifyColumnModal = (tableName: string, col: any) => {
    setSelectedTable(tableName)
    setEditingCol(col)
    setEditColNewName(col.name)
    
    const parsed = parseDbType(col.type)
    setEditColType(parsed.type)
    setEditColLength(parsed.length)
    setEditColNullable(col.nullable)
    setEditColDefault(col.default !== null ? String(col.default) : '')
    setShowColModifyPreview(false)
    setColModifyPreviewSql('')
    
    setDesignerAction('modify_column')
  }

  const generateColModifySql = () => {
    if (!selectedTable || !editingCol) return ''
    const isPostgres = dbOverview?.engine?.toLowerCase() === 'postgres'
    const q = isPostgres ? '"' : '`'

    const escapedTable = `${q}${selectedTable}${q}`
    const escapedCol = `${q}${editingCol.name}${q}`
    
    let dbType = editColType.toUpperCase()
    if (editColType === 'varchar') {
      const len = editColLength === "" ? 255 : Number(editColLength)
      dbType = `VARCHAR(${len})`
    } else if (editColType === 'integer') {
      dbType = 'INT'
    } else if (editColType === 'boolean') {
      dbType = isPostgres ? 'BOOLEAN' : 'TINYINT(1)'
    } else if (editColType === 'decimal') {
      dbType = 'DECIMAL(10,2)'
    }

    const nullability = editColNullable ? 'NULL' : 'NOT NULL'
    const defaultClause = editColDefault !== '' ? ` DEFAULT '${editColDefault.replace(/'/g, "''")}'` : ''

    if (isPostgres) {
      const sqls: string[] = []
      sqls.push(`-- 1. Alter Column Type\nALTER TABLE ${escapedTable} ALTER COLUMN ${escapedCol} TYPE ${dbType} USING ${escapedCol}::${dbType.toLowerCase()};`)
      sqls.push(`-- 2. Alter Column Nullability\nALTER TABLE ${escapedTable} ALTER COLUMN ${escapedCol} ${editColNullable ? 'DROP NOT NULL' : 'SET NOT NULL'};`)
      sqls.push(`-- 3. Alter Column Default\nALTER TABLE ${escapedTable} ALTER COLUMN ${escapedCol} ${editColDefault !== '' ? `SET DEFAULT '${editColDefault.replace(/'/g, "''")}'` : 'DROP DEFAULT'};`)
      if (editColNewName && editColNewName !== editingCol.name) {
        const escapedNewCol = `${q}${editColNewName}${q}`
        sqls.push(`-- 4. Rename Column\nALTER TABLE ${escapedTable} RENAME COLUMN ${escapedCol} TO ${escapedNewCol};`)
      }
      return sqls.join('\n\n')
    } else {
      const sqls: string[] = []
      sqls.push(`ALTER TABLE ${escapedTable} MODIFY COLUMN ${escapedCol} ${dbType} ${nullability}${defaultClause};`)
      if (editColNewName && editColNewName !== editingCol.name) {
        const escapedNewCol = `${q}${editColNewName}${q}`
        sqls.push(`ALTER TABLE ${escapedTable} RENAME COLUMN ${escapedCol} TO ${escapedNewCol};`)
      }
      return sqls.join('\n')
    }
  }

  const handleModifyColumnFormSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const sql = generateColModifySql()
    setColModifyPreviewSql(sql)
    setShowColModifyPreview(true)
  }

  const handleCommitModifyColumn = async () => {
    if (!id || !selectedTable || !editingCol) return

    let payload: any = {
      action: 'modify_column',
      table_name: selectedTable,
      column: {
        name: editingCol.name,
        type: editColType,
        length: editColLength === "" ? 255 : Number(editColLength),
        nullable: editColNullable,
        default_value: editColDefault === "" ? null : editColDefault
      },
      new_name: editColNewName
    }

    setIsActionLoading(true)
    try {
      await databaseAPI.executeDesigner(id, payload)
      toast.success(t('databaseStudio.structure.updateSuccess'))
      setDesignerAction(null)
      loadStudioData()
    } catch (err: any) {
      toast.error(err.response?.data?.error || t('databaseStudio.errors.designerActionFailed'))
    } finally {
      setIsActionLoading(false)
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
        length: newColLength === "" ? 255 : Number(newColLength),
        nullable: newColNullable
      }
    } else if (designerAction === 'create_index') {
      payload.index_name = indexName
      payload.index_columns = indexCols
    }

    setIsActionLoading(true)
    try {
      await databaseAPI.executeDesigner(id, payload)
      toast.success(t('databaseStudio.structure.updateSuccess'))
      setDesignerAction(null)
      loadStudioData()
    } catch (err: any) {
      toast.error(err.response?.data?.error || t('databaseStudio.errors.designerActionFailed'))
    } finally {
      setIsActionLoading(false)
    }
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    toast.success(t('common.copySuccess'))
  }

  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center min-h-[500px] gap-4">
        <LoaderSpinner className="w-12 h-12 text-primary animate-spin" />
        <p className="text-muted-foreground text-sm font-semibold tracking-wide uppercase">{t('databaseStudio.dashboard.connecting')}</p>
      </div>
    )
  }

  const instanceStatus = dbOverview?.status || 'active'
  const isSuspended = instanceStatus === 'suspended'

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-20">
      {/* Studio Header */}
      {!embedded && (
        <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-4 border-b border-border/40 pb-6">
          <div className="space-y-1.5">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-xl bg-primary/10 flex items-center justify-center text-primary border border-primary/20">
                <Database className="w-5 h-5" />
              </div>
              <div>
                <h1 className="text-3xl font-extrabold tracking-tight">
                  {t('databaseStudio.dashboard.title').split(' ')[0]} <span className="text-primary italic">{t('databaseStudio.dashboard.title').split(' ')[1]}</span>
                </h1>
                <p className="text-muted-foreground text-xs uppercase tracking-widest font-bold">{t('databaseStudio.dashboard.subtitle')}</p>
              </div>
            </div>
          </div>

          <div className="flex items-center gap-3 shrink-0">
            <Button
              variant="outline"
              size="sm"
              onClick={loadStudioData}
              disabled={isActionLoading}
              className="gap-2 h-10 cursor-pointer"
              style={{ cursor: 'pointer' }}
            >
              <RefreshCw className={cn("w-4 h-4", isActionLoading && "animate-spin")} />
              {t('databaseStudio.dashboard.actions.syncState')}
            </Button>

            {isSuspended ? (
              <Button
                variant="default"
                size="sm"
                onClick={() => handleToggleStatus(false)}
                disabled={isActionLoading}
                className="gap-2 h-10 bg-emerald-600 hover:bg-emerald-700 text-white font-bold cursor-pointer"
                style={{ cursor: 'pointer' }}
              >
                <Shield className="w-4 h-4" />
                {t('databaseStudio.dashboard.actions.resumeDatabase')}
              </Button>
            ) : (
              <Button
                variant="destructive"
                size="sm"
                onClick={() => handleToggleStatus(true)}
                disabled={isActionLoading}
                className="gap-2 h-10 font-bold cursor-pointer"
                style={{ cursor: 'pointer' }}
              >
                <ShieldAlert className="w-4 h-4 cursor-pointer" style={{ cursor: 'pointer' }} />
                {t('databaseStudio.dashboard.actions.suspendDatabase')}
              </Button>
            )}
          </div>
        </div>
      )}

      {/* Tab Navigation */}
      <div className="flex border-b border-border/60 p-1 bg-muted/20 rounded-xl w-fit">
        {(['dashboard', 'tables', 'structure', 'query', 'backups'] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={cn(
              "px-5 py-2.5 rounded-lg text-sm font-bold capitalize transition-all duration-200 cursor-pointer",
              activeTab === tab 
                ? "bg-background text-primary shadow-sm border border-border/40" 
                : "text-muted-foreground hover:text-foreground"
            )}
            style={{ cursor: 'pointer' }}
          >
            {t(`databaseStudio.tabs.${tab}`)}
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
              <h4 className="font-extrabold uppercase tracking-wide text-destructive text-sm">{t('databaseStudio.dashboard.suspendedTitle')}</h4>
              <p className="text-muted-foreground text-xs mt-1 leading-relaxed">
                {t('databaseStudio.dashboard.suspendedDesc')}
              </p>
            </div>
          </div>
        </Card>
      )}

      {/* Tab Contents */}
      {activeTab === 'dashboard' && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          {/* Left / Main Overview */}
          <div className="lg:col-span-2 space-y-8">
            {/* Metric Grid */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-5">
              <Card className="p-5 flex flex-col gap-1 hover:border-primary/20 transition-all duration-300">
                <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.engine')}</span>
                <span className="text-lg font-extrabold text-foreground capitalize flex items-center gap-1.5">
                  <Server className="w-4 h-4 text-primary" />
                  {dbOverview?.engine || 'MySQL'}
                </span>
              </Card>

              <Card className="p-5 flex flex-col gap-1 hover:border-primary/20 transition-all duration-300">
                <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.version')}</span>
                <span className="text-xs font-mono font-bold truncate mt-1 text-foreground" title={dbOverview?.version}>
                  {dbOverview?.version || 'Unknown'}
                </span>
              </Card>

              <Card className="p-5 flex flex-col gap-1 hover:border-primary/20 transition-all duration-300">
                <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.diskUsed')}</span>
                <span className="text-lg font-extrabold text-foreground flex items-center gap-1.5">
                  <HardDrive className="w-4 h-4 text-primary" />
                  {dbOverview?.size || '0 KB'}
                </span>
              </Card>

              <Card className="p-5 flex flex-col gap-1 hover:border-primary/20 transition-all duration-300">
                <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.tables')}</span>
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
                  {t('databaseStudio.dashboard.metrics.resourceUsageTitle')}
                </h3>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <div className="space-y-2">
                    <div className="flex justify-between text-xs font-bold uppercase tracking-wider text-muted-foreground">
                      <span>{t('databaseStudio.dashboard.metrics.activeConnections')}</span>
                      <span>{metrics.active_connections} / 15</span>
                    </div>
                    <div className="h-2 w-full bg-muted rounded-full overflow-hidden">
                      <div 
                         className="h-full bg-primary transition-all duration-500" 
                         style={{ width: `${Math.min((metrics.active_connections / 15) * 100, 100)}%` }}
                      />
                    </div>
                    <p className="text-[10px] text-muted-foreground italic">{t('databaseStudio.dashboard.metrics.connectionsLimitDesc')}</p>
                  </div>

                  <div className="space-y-2">
                    <div className="flex justify-between text-xs font-bold uppercase tracking-wider text-muted-foreground">
                      <span>{t('databaseStudio.dashboard.metrics.storageUsage')}</span>
                      <span>{dbOverview?.size || '0 KB'} / 1 GB</span>
                    </div>
                    <div className="h-2 w-full bg-muted rounded-full overflow-hidden">
                      <div 
                         className="h-full bg-primary transition-all duration-500" 
                         style={{ width: `${Math.min((metrics.size_kb / 1048576) * 100, 100)}%` }}
                      />
                    </div>
                    <p className="text-[10px] text-muted-foreground italic">{t('databaseStudio.dashboard.metrics.storageLimitDesc')}</p>
                  </div>
                </div>
              </Card>
            )}

            {/* Connection Credentials Card */}
            <Card className="p-6">
              <div className="flex items-center justify-between border-b pb-4 mb-5">
                <h3 className="font-extrabold text-base flex items-center gap-2">
                  <Key className="w-5 h-5 text-primary" />
                  {t('databaseStudio.dashboard.credentials.title')}
                </h3>
                <Button
                  variant="outline"
                  size="xs"
                  onClick={handleRotateCredentials}
                  disabled={isActionLoading || isSuspended}
                  className="font-bold border-primary/20 hover:border-primary shrink-0 gap-1.5 h-8 text-xs cursor-pointer"
                  style={{ cursor: 'pointer' }}
                >
                  <RefreshCw className="w-3.5 h-3.5" />
                  {t('databaseStudio.dashboard.actions.rotateCredentials')}
                </Button>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
                <div className="space-y-1.5">
                  <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.credentials.host')}</span>
                  <div className="flex items-center justify-between p-3 rounded-lg border bg-muted/10 font-mono text-xs">
                    <span className="truncate">{dbOverview?.host || 'localhost'}</span>
                    <button onClick={() => copyToClipboard(dbOverview?.host || '')} className="text-muted-foreground hover:text-foreground cursor-pointer" style={{ cursor: 'pointer' }}>
                      <Copy className="w-3.5 h-3.5 cursor-pointer" style={{ cursor: 'pointer' }} />
                    </button>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.credentials.port')}</span>
                  <div className="flex items-center justify-between p-3 rounded-lg border bg-muted/10 font-mono text-xs">
                    <span>{dbOverview?.port || 3306}</span>
                    <button onClick={() => copyToClipboard(String(dbOverview?.port || 3306))} className="text-muted-foreground hover:text-foreground cursor-pointer" style={{ cursor: 'pointer' }}>
                      <Copy className="w-3.5 h-3.5 cursor-pointer" style={{ cursor: 'pointer' }} />
                    </button>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.credentials.databaseName')}</span>
                  <div className="flex items-center justify-between p-3 rounded-lg border bg-muted/10 font-mono text-xs">
                    <span className="truncate">{dbOverview?.database || ''}</span>
                    <button onClick={() => copyToClipboard(dbOverview?.database || '')} className="text-muted-foreground hover:text-foreground cursor-pointer" style={{ cursor: 'pointer' }}>
                      <Copy className="w-3.5 h-3.5 cursor-pointer" style={{ cursor: 'pointer' }} />
                    </button>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.credentials.username')}</span>
                  <div className="flex items-center justify-between p-3 rounded-lg border bg-muted/10 font-mono text-xs">
                    <span className="truncate">{dbOverview?.username || ''}</span>
                    <button onClick={() => copyToClipboard(dbOverview?.username || '')} className="text-muted-foreground hover:text-foreground cursor-pointer" style={{ cursor: 'pointer' }}>
                      <Copy className="w-3.5 h-3.5 cursor-pointer" style={{ cursor: 'pointer' }} />
                    </button>
                  </div>
                </div>

                <div className="space-y-1.5 md:col-span-2">
                  <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.credentials.password')}</span>
                  <div className="flex items-center justify-between p-3 rounded-lg border bg-muted/10 font-mono text-xs">
                    <input 
                      type={revealPassword ? "text" : "password"} 
                      value={dbOverview?.password || ''} 
                      readOnly 
                      className="bg-transparent border-none outline-none focus:ring-0 flex-1 min-w-0 pr-4"
                    />
                    <div className="flex items-center gap-3 shrink-0">
                      <button onClick={() => setRevealPassword(!revealPassword)} className="text-muted-foreground hover:text-foreground cursor-pointer" style={{ cursor: 'pointer' }}>
                        {revealPassword ? <EyeOff className="w-3.5 h-3.5 cursor-pointer" style={{ cursor: 'pointer' }} /> : <Eye className="w-3.5 h-3.5 cursor-pointer" style={{ cursor: 'pointer' }} />}
                      </button>
                      <button onClick={() => copyToClipboard(dbOverview?.password || '')} className="text-muted-foreground hover:text-foreground cursor-pointer" style={{ cursor: 'pointer' }}>
                        <Copy className="w-3.5 h-3.5 cursor-pointer" style={{ cursor: 'pointer' }} />
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </Card>
          </div>

          {/* Right Sidebar - Admin Controls */}
          <div className="space-y-8">
            <Card className="p-5 space-y-4">
              <h4 className="font-extrabold text-sm uppercase tracking-wide border-b pb-2 flex items-center gap-2">
                <Activity className="w-4.5 h-4.5" />
                {t('databaseStudio.dashboard.metrics.status')} & {t('common.actions')}
              </h4>
              
              <div className="space-y-3">
                <div className="flex justify-between items-center text-xs">
                  <span className="font-bold text-muted-foreground uppercase tracking-wider">{t('databaseStudio.dashboard.metrics.status')}:</span>
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
                    className="w-full text-xs font-bold gap-2 hover:bg-muted cursor-pointer"
                    style={{ cursor: 'pointer' }}
                    onClick={handleRestartPool}
                    disabled={isActionLoading || isSuspended}
                  >
                    <RefreshCw className="w-3.5 h-3.5" />
                    {t('databaseStudio.dashboard.actions.testConnection')}
                  </Button>
                </div>
              </div>
            </Card>
          </div>
        </div>
      )}

      {activeTab === 'tables' && (
        <div className="grid grid-cols-1 lg:grid-cols-4 gap-6 items-stretch animate-in fade-in duration-300">
          {/* Left Column: Table List Sidebar */}
          <Card className="lg:col-span-1 flex flex-col overflow-hidden border-none shadow-xl bg-card/95 ring-1 ring-white/5 p-4 gap-3">
            <div className="flex items-center justify-between px-2 pt-1 border-b border-border/40 pb-2">
              <span className="text-[10px] font-black uppercase tracking-wider text-muted-foreground">{t('databaseStudio.tables.sidebarTitle')} ({schemaData.length})</span>
            </div>
            
            {!isSuspended && schemaData.length > 0 && (
              <div className="relative">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground/60" />
                <Input
                  placeholder={t('databaseStudio.tables.searchPlaceholder')}
                  value={tableSearch}
                  onChange={(e) => setTableSearch(e.target.value)}
                  className="pl-9 h-10 text-xs font-semibold rounded-xl bg-background/50 border-border/70 hover:border-primary/30 focus-visible:ring-primary/25"
                />
              </div>
            )}
            
            <div className="flex-1 overflow-y-auto space-y-1.5 scrollbar-thin max-h-[500px] pr-1">
              {isSuspended ? (
                <div className="text-center py-8 text-xs text-muted-foreground/50 italic font-semibold">{t('databaseStudio.dashboard.suspendedTitle')}</div>
              ) : schemaData.length === 0 ? (
                <div className="text-center py-8 text-xs text-muted-foreground/50 italic font-semibold">{t('databaseStudio.tables.noTablesFound')}</div>
              ) : filteredTables.length === 0 ? (
                <div className="text-center py-8 text-xs text-muted-foreground/50 italic font-semibold">{t('databaseStudio.tables.noMatches')}</div>
              ) : (
                filteredTables.map(table => (
                  <button
                    key={table.name}
                    onClick={() => {
                      setSelectedTable(table.name)
                      setTablePage(1)
                    }}
                    className={cn(
                      "w-full text-left px-3.5 py-2.5 rounded-lg border text-xs font-semibold flex items-center justify-between transition-all duration-200 group cursor-pointer",
                      selectedTable === table.name
                        ? 'bg-primary/10 border-primary/20 text-primary shadow-sm'
                        : 'border-transparent text-muted-foreground/80 hover:bg-muted/40 hover:text-foreground hover:border-border/50'
                    )}
                    style={{ cursor: 'pointer' }}
                  >
                    <div className="flex items-center gap-2 truncate">
                      <Table className={cn("w-3.5 h-3.5 shrink-0", selectedTable === table.name ? "text-primary" : "text-muted-foreground/60 group-hover:text-foreground")} />
                      <span className="truncate pr-1 tracking-tight">{table.name}</span>
                    </div>
                    {table.rows != null && (
                      <span className={cn(
                        "text-[9px] font-mono px-1.5 py-0.5 rounded-md",
                        selectedTable === table.name ? 'bg-primary/20 text-primary' : 'bg-muted text-muted-foreground/50'
                      )}>
                        {table.rows}
                      </span>
                    )}
                  </button>
                ))
              )}
            </div>
          </Card>

          {/* Right Column: Data Grid */}
          <Card className="lg:col-span-3 p-6 flex flex-col overflow-hidden">
            <div className="flex items-start sm:items-center justify-between gap-4 border-b pb-4 mb-5">
              <div className="flex items-center gap-3">
                <Table className="w-5 h-5 text-primary" />
                <div>
                  <h3 className="font-extrabold text-base">{selectedTable || t('databaseStudio.tables.noTableSelected')}</h3>
                  <p className="text-muted-foreground text-xs">{t('databaseStudio.tables.tableDesc')}</p>
                </div>
              </div>
              {selectedTable && !isSuspended && (
                <Button
                  onClick={() => {
                    const cols = schemaData.find(t => t.name === selectedTable)?.columns || []
                    const initialData: Record<string, any> = {}
                    cols.forEach((c: any) => {
                      if (c.key === 'PRI') return
                      initialData[c.name] = c.default !== null ? c.default : ''
                    })
                    setInsertFormData(initialData)
                    setShowInsertModal(true)
                  }}
                  className="font-bold gap-1.5 h-10 px-4 rounded-xl bg-primary hover:bg-primary/90 text-primary-foreground shadow-sm shrink-0 cursor-pointer"
                  style={{ cursor: 'pointer' }}
                >
                  <Plus className="w-4 h-4" />
                  {t('databaseStudio.tables.insertRow')}
                </Button>
              )}
            </div>

            {selectedTable && !isSuspended && (
              <div className="grid grid-cols-3 gap-4 mb-5 animate-in fade-in duration-300">
                <div className="p-3.5 bg-muted/20 border border-border/40 rounded-xl flex flex-col hover:border-primary/20 transition-all">
                  <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.tables.stats.rows')}</span>
                  <span className="text-lg font-extrabold text-foreground mt-0.5">
                    {schemaData.find(t => t.name === selectedTable)?.rows ?? tableTotal}
                  </span>
                </div>
                <div className="p-3.5 bg-muted/20 border border-border/40 rounded-xl flex flex-col hover:border-primary/20 transition-all">
                  <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.tables.stats.cols')}</span>
                  <span className="text-lg font-extrabold text-foreground mt-0.5">
                    {schemaData.find(t => t.name === selectedTable)?.columns?.length || 0}
                  </span>
                </div>
                <div className="p-3.5 bg-muted/20 border border-border/40 rounded-xl flex flex-col hover:border-primary/20 transition-all">
                  <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.engine')}</span>
                  <span className="text-lg font-extrabold text-foreground mt-0.5 capitalize">
                    {dbOverview?.engine || 'MySQL'}
                  </span>
                </div>
              </div>
            )}

            {isSuspended ? (
              <div className="py-12 text-center text-muted-foreground text-sm font-semibold uppercase tracking-wide">
                {t('databaseStudio.dashboard.suspendedWarning')}
              </div>
            ) : !selectedTable ? (
              <div className="py-12 border border-dashed rounded-xl flex flex-col items-center justify-center text-center gap-3 bg-muted/5">
                <Table className="w-8 h-8 text-muted-foreground" />
                <div className="space-y-1">
                  <h4 className="font-bold text-sm">{t('databaseStudio.tables.noTableSelected')}</h4>
                  <p className="text-xs text-muted-foreground">{t('databaseStudio.tables.noTableSelectedDesc')}</p>
                </div>
              </div>
            ) : tableData ? (
              <div className="space-y-4 flex-1 flex flex-col min-h-0">
                <div className="overflow-x-auto border border-border/80 rounded-xl bg-background/30 max-h-[420px] flex-1">
                  <table className="w-full text-left border-collapse text-xs font-medium">
                    <thead>
                      <tr className="bg-muted/30 border-b border-border/80 text-[10px] font-bold uppercase tracking-widest text-muted-foreground sticky top-0 backdrop-blur-md z-10">
                        <th className="py-3.5 px-4 w-12 text-center bg-muted/30">{t('databaseStudio.tables.actionHeader')}</th>
                        {tableData.columns.map((col: string) => (
                          <th key={col} className="py-3.5 px-4 font-mono font-semibold bg-muted/30">{col}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {tableData.rows.length === 0 ? (
                        <tr>
                          <td colSpan={tableData.columns.length + 1} className="py-10 text-center text-muted-foreground italic font-semibold">
                            {t('databaseStudio.tables.tableEmpty')}
                          </td>
                        </tr>
                      ) : (
                        tableData.rows.map((row: any, idx: number) => {
                          const pkColumn = tableData.columns.find((c: string) => 
                            c.toLowerCase() === 'id' || c.toLowerCase() === 'uid' || c.toLowerCase() === 'uuid'
                          ) || tableData.columns[0]

                          return (
                            <tr key={idx} className="border-b border-border/40 hover:bg-muted/15 transition-colors">
                              <td className="py-3.5 px-4 text-center shrink-0">
                                <DropdownMenu>
                                  <DropdownMenuTrigger>
                                    <Button variant="ghost" size="icon" className="h-8 w-8 hover:bg-muted/50 cursor-pointer" style={{ cursor: 'pointer' }}>
                                      <MoreHorizontal className="w-4 h-4" />
                                    </Button>
                                  </DropdownMenuTrigger>
                                  <DropdownMenuContent align="center" className="w-32 bg-card/98 border border-border/80 rounded-xl shadow-xl backdrop-blur-xl">
                                    <DropdownMenuItem onClick={() => openEditRowModal(row)} className="gap-2 cursor-pointer text-xs font-bold" style={{ cursor: 'pointer' }}>
                                      <Pencil className="w-3.5 h-3.5 text-muted-foreground" />
                                      {t('common.edit')}
                                    </DropdownMenuItem>
                                    <DropdownMenuItem onClick={() => handleDeleteRow(row, pkColumn)} className="text-destructive gap-2 cursor-pointer text-xs font-bold" style={{ cursor: 'pointer' }}>
                                      <Trash2 className="w-3.5 h-3.5" />
                                      {t('common.delete')}
                                    </DropdownMenuItem>
                                  </DropdownMenuContent>
                                </DropdownMenu>
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
                  <div className="flex items-center justify-between border-t border-border/40 pt-4 mt-auto">
                    <span className="text-xs text-muted-foreground font-semibold">
                      {t('databaseStudio.tables.showingRows', { start: (tablePage - 1) * tableLimit + 1, end: Math.min(tablePage * tableLimit, tableTotal), total: tableTotal })}
                    </span>
                    <div className="flex items-center gap-2">
                      <Button 
                        variant="outline" 
                        size="xs" 
                        onClick={() => setTablePage(prev => Math.max(prev - 1, 1))}
                        disabled={tablePage === 1}
                        className="font-bold h-8 text-xs px-3 rounded-lg cursor-pointer"
                        style={{ cursor: 'pointer' }}
                      >
                        {t('common.previous')}
                      </Button>
                      <Button 
                        variant="outline" 
                        size="xs" 
                        onClick={() => setTablePage(prev => prev + 1)}
                        disabled={tablePage * tableLimit >= tableTotal}
                        className="font-bold h-8 text-xs px-3 rounded-lg cursor-pointer"
                        style={{ cursor: 'pointer' }}
                      >
                        {t('common.next')}
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <div className="py-10 text-center text-muted-foreground flex-1 flex items-center justify-center">{t('databaseStudio.tables.loadingRows')}</div>
            )}
          </Card>

          {/* Visual Insert Row Modal */}
          <Dialog open={showInsertModal} onOpenChange={(open: boolean) => !open && setShowInsertModal(false)}>
            <DialogContent className="sm:max-w-md bg-card/98 border border-border/80 rounded-xl shadow-2xl backdrop-blur-xl max-h-[85vh] overflow-y-auto">
              <DialogHeader className="pb-2 border-b border-border/40">
                <DialogTitle className="text-lg font-extrabold flex items-center gap-2 text-foreground/90">
                  <PlusCircle className="w-5 h-5 text-primary" />
                  {t('databaseStudio.tables.insertModal.title')}
                </DialogTitle>
                <DialogDescription className="text-xs text-muted-foreground">
                  {t('databaseStudio.tables.insertModal.desc')} — <span className="font-mono text-primary font-semibold">{selectedTable}</span>
                </DialogDescription>
              </DialogHeader>
              <form onSubmit={handleInsertRowSubmit} className="space-y-4 pt-3">
                <div className="space-y-3.5 max-h-[50vh] overflow-y-auto pr-1">
                  {(schemaData.find(t => t.name === selectedTable)?.columns || []).map((col: any) => {
                    const isPK = col.key === 'PRI'
                    const isNullable = col.nullable
                    const typeLower = col.type.toLowerCase()

                    return (
                      <div key={col.name} className="space-y-1.5">
                        <div className="flex items-center justify-between">
                          <Label htmlFor={`insert_${col.name}`} className="text-xs font-bold text-foreground/90 flex items-center gap-2">
                            <span className="font-mono">{col.name}</span>
                            <span className="text-[10px] text-muted-foreground font-normal">({col.type})</span>
                          </Label>
                          <div className="flex items-center gap-1.5">
                            {isPK && (
                              <span className="px-1.5 py-0.5 rounded bg-primary/10 text-primary border border-primary/20 text-[9px] font-black uppercase">
                                PK
                              </span>
                            )}
                            {!isNullable ? (
                              <span className="text-[9px] font-extrabold uppercase text-amber-500/80 bg-amber-500/5 px-1.5 py-0.5 rounded border border-amber-500/10">
                                Required
                              </span>
                            ) : (
                              <span className="text-[9px] font-bold uppercase text-muted-foreground/60 bg-muted/10 px-1.5 py-0.5 rounded border border-border/40">
                                Optional
                              </span>
                            )}
                          </div>
                        </div>

                        {isPK ? (
                          <Input
                            id={`insert_${col.name}`}
                            disabled
                            value={insertFormData[col.name] || ''}
                            placeholder="Auto-incrementing ID"
                            className="h-10 rounded-xl bg-muted/40 border-border/40 font-mono text-xs cursor-not-allowed"
                          />
                        ) : typeLower.includes('bool') || typeLower.includes('tinyint(1)') ? (
                          <select
                            id={`insert_${col.name}`}
                            value={String(insertFormData[col.name] ?? '')}
                            onChange={(e) => setInsertFormData(prev => ({ ...prev, [col.name]: e.target.value }))}
                            className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold outline-none focus:border-primary/50 cursor-pointer"
                            style={{ cursor: 'pointer' }}
                          >
                            <option value="">-- Select Boolean --</option>
                            <option value="true">True (Yes)</option>
                            <option value="false">False (No)</option>
                          </select>
                        ) : typeLower.includes('timestamp') || typeLower.includes('datetime') || typeLower.includes('date') ? (
                          <Input
                            id={`insert_${col.name}`}
                            key={selectedTable + '_' + col.name}
                            type={typeLower.includes('date') && !typeLower.includes('time') ? "date" : "datetime-local"}
                            defaultValue={insertFormData[col.name] || ''}
                            onChange={(e) => setInsertFormData(prev => ({ ...prev, [col.name]: e.target.value }))}
                            required={!isNullable}
                            className="h-10 rounded-xl bg-background/50 text-xs font-mono"
                          />
                        ) : typeLower.includes('text') ? (
                          <textarea
                            id={`insert_${col.name}`}
                            value={insertFormData[col.name] || ''}
                            onChange={(e) => setInsertFormData(prev => ({ ...prev, [col.name]: e.target.value }))}
                            required={!isNullable}
                            rows={3}
                            placeholder={`Enter text data...`}
                            className="w-full p-3 rounded-xl border border-border/70 bg-background/50 text-xs font-mono outline-none focus:border-primary/50"
                          />
                        ) : (
                          <Input
                            id={`insert_${col.name}`}
                            type={typeLower.includes('int') || typeLower.includes('decimal') || typeLower.includes('float') || typeLower.includes('double') ? "number" : "text"}
                            value={insertFormData[col.name] || ''}
                            onChange={(e) => setInsertFormData(prev => ({ ...prev, [col.name]: e.target.value }))}
                            required={!isNullable}
                            placeholder={isNullable ? "Optional value" : "Required value"}
                            className="h-10 rounded-xl bg-background/50 text-xs font-mono"
                          />
                        )}
                      </div>
                    )
                  })}
                </div>

                <div className="flex gap-2.5 pt-2 border-t border-border/40">
                  <Button type="submit" disabled={isActionLoading} className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {isActionLoading ? t('common.executing') : t('databaseStudio.tables.insertModal.submit')}
                  </Button>
                  <Button type="button" onClick={() => setShowInsertModal(false)} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {t('common.cancel')}
                  </Button>
                </div>
              </form>
            </DialogContent>
          </Dialog>

          {/* Visual Edit Row Modal */}
          <Dialog open={showEditModal} onOpenChange={(open: boolean) => !open && setShowEditModal(false)}>
            <DialogContent className="sm:max-w-md bg-card/98 border border-border/80 rounded-xl shadow-2xl backdrop-blur-xl max-h-[85vh] overflow-y-auto">
              <DialogHeader className="pb-2 border-b border-border/40">
                <DialogTitle className="text-lg font-extrabold flex items-center gap-2 text-foreground/90">
                  <Pencil className="w-5 h-5 text-primary" />
                  {showRowEditPreview ? t('databaseStudio.tables.editModal.previewTitle') : t('databaseStudio.tables.editModal.title')}
                </DialogTitle>
                <DialogDescription className="text-xs text-muted-foreground">
                  {showRowEditPreview ? t('databaseStudio.tables.editModal.previewDesc') : t('databaseStudio.tables.editModal.desc')} — <span className="font-mono text-primary font-semibold">{selectedTable}</span>
                </DialogDescription>
              </DialogHeader>

              {!showRowEditPreview ? (
                <form onSubmit={handleEditRowFormSubmit} className="space-y-4 pt-3">
                  <div className="space-y-3.5 max-h-[50vh] overflow-y-auto pr-1">
                    {(schemaData.find(t => t.name === selectedTable)?.columns || []).map((col: any) => {
                      const pkColumn = tableData?.columns.find((c: string) => 
                        c.toLowerCase() === 'id' || c.toLowerCase() === 'uid' || c.toLowerCase() === 'uuid'
                      ) || tableData?.columns[0]
                      const isPK = col.name === pkColumn
                      const isNullable = col.nullable
                      const typeLower = col.type.toLowerCase()

                      return (
                        <div key={col.name} className="space-y-1.5">
                          <div className="flex items-center justify-between">
                            <Label htmlFor={`edit_${col.name}`} className="text-xs font-bold text-foreground/90 flex items-center gap-2">
                              <span className="font-mono">{col.name}</span>
                              <span className="text-[10px] text-muted-foreground font-normal">({col.type})</span>
                            </Label>
                            <div className="flex items-center gap-1.5">
                              {isPK && (
                                <span className="px-1.5 py-0.5 rounded bg-primary/10 text-primary border border-primary/20 text-[9px] font-black uppercase">
                                  PK
                                </span>
                              )}
                              {!isNullable ? (
                                <span className="text-[9px] font-extrabold uppercase text-amber-500/80 bg-amber-500/5 px-1.5 py-0.5 rounded border border-amber-500/10">
                                  Required
                                </span>
                              ) : (
                                <span className="text-[9px] font-bold uppercase text-muted-foreground/60 bg-muted/10 px-1.5 py-0.5 rounded border border-border/40">
                                  Optional
                                </span>
                              )}
                            </div>
                          </div>

                          {isPK ? (
                            <Input
                              id={`edit_${col.name}`}
                              disabled
                              value={editFormData[col.name] || ''}
                              className="h-10 rounded-xl bg-muted/40 border-border/40 font-mono text-xs cursor-not-allowed"
                            />
                          ) : typeLower.includes('bool') || typeLower.includes('tinyint(1)') ? (
                            <select
                              id={`edit_${col.name}`}
                              value={String(editFormData[col.name] ?? '')}
                              onChange={(e) => setEditFormData(prev => ({ ...prev, [col.name]: e.target.value }))}
                              className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold outline-none focus:border-primary/50 cursor-pointer"
                              style={{ cursor: 'pointer' }}
                            >
                              <option value="">-- Select Boolean --</option>
                              <option value="true">True (Yes)</option>
                              <option value="false">False (No)</option>
                            </select>
                          ) : typeLower.includes('timestamp') || typeLower.includes('datetime') || typeLower.includes('date') ? (
                            <Input
                              id={`edit_${col.name}`}
                              key={editingRow ? `${editingRow[pkColumn]}_${col.name}` : col.name}
                              type={typeLower.includes('date') && !typeLower.includes('time') ? "date" : "datetime-local"}
                              defaultValue={editFormData[col.name] || ''}
                              onChange={(e) => setEditFormData(prev => ({ ...prev, [col.name]: e.target.value }))}
                              required={!isNullable}
                              className="h-10 rounded-xl bg-background/50 text-xs font-mono"
                            />
                          ) : typeLower.includes('text') ? (
                            <textarea
                              id={`edit_${col.name}`}
                              value={editFormData[col.name] || ''}
                              onChange={(e) => setEditFormData(prev => ({ ...prev, [col.name]: e.target.value }))}
                              required={!isNullable}
                              rows={3}
                              placeholder={`Enter text data...`}
                              className="w-full p-3 rounded-xl border border-border/70 bg-background/50 text-xs font-mono outline-none focus:border-primary/50"
                            />
                          ) : (
                            <Input
                              id={`edit_${col.name}`}
                              type={typeLower.includes('int') || typeLower.includes('decimal') || typeLower.includes('float') || typeLower.includes('double') ? "number" : "text"}
                              value={editFormData[col.name] || ''}
                              onChange={(e) => setEditFormData(prev => ({ ...prev, [col.name]: e.target.value }))}
                              required={!isNullable}
                              placeholder={isNullable ? "Optional value" : "Required value"}
                              className="h-10 rounded-xl bg-background/50 text-xs font-mono"
                            />
                          )}
                        </div>
                      )
                    })}
                  </div>

                  <div className="flex gap-2.5 pt-2 border-t border-border/40">
                    <Button type="submit" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                      {t('databaseStudio.tables.editModal.submit')}
                    </Button>
                    <Button type="button" onClick={() => setShowEditModal(false)} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                      {t('common.cancel')}
                    </Button>
                  </div>
                </form>
              ) : (
                <div className="space-y-4 pt-3">
                  <div className="rounded-xl border border-border/80 bg-background/50 overflow-hidden">
                    <div className="flex items-center justify-between bg-muted/30 px-4 py-2 border-b border-border/80">
                      <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">SQL Script</span>
                      <button 
                        type="button" 
                        onClick={() => copyToClipboard(rowEditPreviewSql)} 
                        className="p-1 hover:bg-muted rounded text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                        style={{ cursor: 'pointer' }}
                      >
                        <Copy className="w-3.5 h-3.5" />
                      </button>
                    </div>
                    <textarea
                      readOnly
                      value={rowEditPreviewSql}
                      className="w-full h-44 p-4 font-mono text-xs bg-transparent border-none outline-none resize-none leading-relaxed text-foreground/90 select-all"
                    />
                  </div>

                  <div className="flex gap-2.5 pt-2 border-t border-border/40">
                    <Button onClick={handleEditRowSubmit} disabled={isActionLoading} className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                      {isActionLoading ? t('common.executing') : t('databaseStudio.tables.editModal.commitBtn')}
                    </Button>
                    <Button onClick={() => setShowRowEditPreview(false)} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                      {t('databaseStudio.tables.editModal.backBtn')}
                    </Button>
                  </div>
                </div>
              )}
            </DialogContent>
          </Dialog>
        </div>
      )}      {activeTab === 'structure' && (
        <>
          <div className="border border-border/80 shadow-2xl overflow-hidden flex flex-col md:flex-row h-[600px] bg-card/40 backdrop-blur-md rounded-xl animate-in fade-in duration-300">
            {/* Left Sidebar: macOS Finder Style Sidebar */}
            <div className="w-full md:w-80 border-r border-border/60 bg-muted/5 flex flex-col h-full shrink-0">
              {/* Sidebar Header with Title & Action */}
              <div className="p-4 border-b border-border/40 space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">
                    {t('databaseStudio.tables.title') || 'Tables'}
                  </span>
                  <Button
                    size="xs"
                    onClick={() => setDesignerAction('create_table')}
                    disabled={isActionLoading || isSuspended}
                    className="font-bold gap-1 h-6 px-2 text-[10px] rounded-lg bg-primary/10 hover:bg-primary/20 text-primary border border-primary/20 hover:border-primary/40 shadow-none transition-colors cursor-pointer"
                    style={{ cursor: 'pointer' }}
                  >
                    <Plus className="w-3 h-3" />
                    {t('databaseStudio.structure.createTableDialog.submitBtn')}
                  </Button>
                </div>
                
                {/* Search Bar for Tables */}
                {!isSuspended && schemaData.length > 0 && (
                  <div className="relative">
                    <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground/50" />
                    <Input
                      type="text"
                      placeholder={t('databaseStudio.tables.searchPlaceholder') || 'Search tables...'}
                      value={structureSearch}
                      onChange={(e) => setStructureSearch(e.target.value)}
                      className="h-8 pl-8 pr-3 text-[11px] rounded-lg bg-background/50 border-border/60 focus:bg-background/80"
                    />
                  </div>
                )}
              </div>

              {/* Sidebar Scrollable Content */}
              <div className="flex-1 overflow-y-auto p-2 space-y-0.5 scrollbar-thin">
                {isSuspended ? (
                  <div className="py-12 text-center text-[10px] font-bold text-muted-foreground uppercase tracking-wider">
                    {t('databaseStudio.dashboard.suspendedWarning')}
                  </div>
                ) : schemaData.length === 0 ? (
                  <div className="flex flex-col items-center justify-center py-16 text-center text-muted-foreground gap-2">
                    <DatabaseZap className="w-8 h-8 text-muted-foreground/30" />
                    <span className="text-[10px] font-bold uppercase tracking-wider">{t('common.empty')}</span>
                  </div>
                ) : (() => {
                  const filteredTables = schemaData.filter(table => 
                    table.name.toLowerCase().includes(structureSearch.toLowerCase())
                  )
                  if (filteredTables.length === 0) {
                    return (
                      <div className="py-12 text-center text-xs text-muted-foreground">
                        {t('databaseStudio.tables.noTablesMatch', { search: structureSearch })}
                      </div>
                    )
                  }
                  return filteredTables.map((table: any) => {
                    const isSelected = selectedTable === table.name
                    return (
                      <button
                        key={table.name}
                        onClick={() => {
                          setSelectedTable(table.name)
                        }}
                        className={cn(
                          "w-full text-left py-2 px-3 rounded-lg text-xs font-mono font-semibold transition-all duration-150 cursor-pointer flex items-center justify-between group border",
                          isSelected
                            ? "bg-primary/10 border-primary/20 text-primary shadow-sm"
                            : "border-transparent hover:bg-muted/10 text-muted-foreground hover:text-foreground"
                        )}
                        style={{ cursor: 'pointer' }}
                      >
                        <span className="flex items-center gap-2 truncate pr-1">
                          <Table className={cn("w-3.5 h-3.5 transition-colors", isSelected ? "text-primary animate-pulse" : "text-muted-foreground/60 group-hover:text-foreground/80")} />
                          {table.name}
                        </span>
                        <span className={cn(
                          "text-[9px] px-1.5 py-0.5 rounded font-mono font-bold border shrink-0 transition-all",
                          isSelected 
                            ? "bg-primary/20 text-primary border-primary/30" 
                            : "bg-muted text-muted-foreground/50 border-muted-foreground/10 group-hover:text-muted-foreground"
                        )}>
                          {table.columns?.length || 0}
                        </span>
                      </button>
                    )
                  })
                })()}
              </div>
            </div>

            {/* Right Pane: macOS Finder Detail View */}
            <div className="flex-1 flex flex-col h-full min-w-0 bg-background/5 overflow-hidden">
              {isSuspended ? (
                <div className="flex-1 flex items-center justify-center p-6 text-center text-muted-foreground">
                  <div className="text-sm font-semibold uppercase tracking-wide">
                    {t('databaseStudio.dashboard.suspendedWarning')}
                  </div>
                </div>
              ) : selectedTable && schemaData.find(t => t.name === selectedTable) ? (
                (() => {
                  const table = schemaData.find(t => t.name === selectedTable)
                  return (
                    <div key={table.name} className="flex-1 flex flex-col h-full overflow-hidden animate-in fade-in slide-in-from-right-2 duration-300 ease-out">
                      {/* Pane Header */}
                      <div className="flex items-center justify-between border-b border-border/40 p-4 shrink-0 bg-muted/5">
                        <div className="flex items-center gap-3">
                          <span className="font-mono font-bold text-sm text-foreground/90 flex items-center gap-2">
                            <Table className="w-4 h-4 text-primary" />
                            {table.name}
                          </span>
                          
                          <Button
                            variant="outline"
                            size="xs"
                            onClick={() => {
                              setSelectedTable(table.name)
                              setDesignerAction('add_column')
                            }}
                            className="h-7 text-[11px] font-bold gap-1 rounded-lg px-2.5 bg-primary/10 hover:bg-primary/20 text-primary border border-primary/20 hover:border-primary/45 shadow-none transition-colors cursor-pointer"
                            style={{ cursor: 'pointer' }}
                          >
                            <Plus className="w-3.5 h-3.5" />
                            {t('databaseStudio.structure.addColumn')}
                          </Button>
                        </div>

                        <Button
                          variant="ghost"
                          size="xs"
                          onClick={() => handleDeleteTable(table.name)}
                          className="h-7 text-[11px] font-bold gap-1 rounded-lg px-2.5 text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors cursor-pointer"
                          style={{ cursor: 'pointer' }}
                          title={t('databaseStudio.structure.actions.dropTable')}
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                          {t('databaseStudio.structure.actions.dropTable')}
                        </Button>
                      </div>

                      {/* Pane Scrollable Table Grid */}
                      <div className="flex-1 overflow-y-auto p-4 scrollbar-thin">
                        <div className="border border-border/60 rounded-xl overflow-hidden bg-background/20">
                          <table className="w-full text-left border-collapse text-xs font-medium">
                            <thead>
                              <tr className="border-b border-border/40 bg-muted/20 text-[10px] font-bold uppercase tracking-wider text-muted-foreground sticky top-0 backdrop-blur-md z-10">
                                <th className="py-3 px-4 bg-muted/20">{t('databaseStudio.structure.nameHeader')}</th>
                                <th className="py-3 px-4 bg-muted/20">{t('databaseStudio.structure.typeHeader')}</th>
                                <th className="py-3 px-4 text-center bg-muted/20">{t('databaseStudio.structure.createTableDialog.nullableLabel')}</th>
                                <th className="py-3 px-4 text-center bg-muted/20">{t('databaseStudio.structure.keyHeader')}</th>
                                <th className="py-3 px-4 bg-muted/20">{t('databaseStudio.structure.defaultHeader')}</th>
                                <th className="py-3 px-4 text-right bg-muted/20">{t('databaseStudio.tables.actionHeader')}</th>
                              </tr>
                            </thead>
                            <tbody>
                              {table.columns.map((col: any) => (
                                <tr key={col.name} className="border-b border-border/15 hover:bg-muted/5 transition-colors">
                                  <td className="py-3 px-4 font-mono font-semibold text-foreground/90">{col.name}</td>
                                  <td className="py-3 px-4 font-mono text-primary/80">{col.type}</td>
                                  <td className="py-3 px-4 text-center">
                                    <span className={cn(
                                      "px-2 py-0.5 rounded text-[10px] font-bold border",
                                      col.nullable 
                                        ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/20" 
                                        : "bg-amber-500/10 text-amber-500 border-amber-500/20"
                                    )}>
                                      {col.nullable ? t('common.yes').toUpperCase() : t('common.no').toUpperCase()}
                                    </span>
                                  </td>
                                  <td className="py-3 px-4 text-center">
                                    {col.key === 'PRI' && (
                                      <span className="px-2 py-0.5 rounded bg-primary/10 text-primary border border-primary/20 text-[10px] font-extrabold uppercase tracking-wide">
                                        PK
                                      </span>
                                    )}
                                    {col.key === 'UNI' && (
                                      <span className="px-2 py-0.5 rounded bg-amber-500/10 text-amber-600 border border-amber-500/20 text-[10px] font-extrabold uppercase tracking-wide" title={t('databaseStudio.structure.uniqueConstraint')}>
                                        UQ
                                      </span>
                                    )}
                                    {col.key === 'MUL' && (
                                      <span className="px-2 py-0.5 rounded bg-sky-500/10 text-sky-600 border border-sky-500/20 text-[10px] font-extrabold uppercase tracking-wide" title={t('databaseStudio.structure.indexedColumn')}>
                                        IDX
                                      </span>
                                    )}
                                  </td>
                                  <td className="py-3 px-4 font-mono text-muted-foreground">{col.default === null ? <span className="text-muted-foreground/30 italic">NULL</span> : String(col.default)}</td>
                                  <td className="py-3 px-4 text-right">
                                    <DropdownMenu>
                                      <DropdownMenuTrigger>
                                        <Button variant="ghost" size="icon" className="h-8 w-8 hover:bg-muted/50 cursor-pointer" style={{ cursor: 'pointer' }}>
                                          <MoreHorizontal className="w-4 h-4" />
                                        </Button>
                                      </DropdownMenuTrigger>
                                      <DropdownMenuContent align="end" className="w-36 bg-card/98 border border-border/80 rounded-xl shadow-xl backdrop-blur-xl">
                                        <DropdownMenuItem onClick={() => openModifyColumnModal(table.name, col)} className="gap-2 cursor-pointer text-xs font-bold" style={{ cursor: 'pointer' }}>
                                          <Pencil className="w-3.5 h-3.5 text-muted-foreground" />
                                          {t('databaseStudio.structure.actions.modifyColumn') || 'Modify Column'}
                                        </DropdownMenuItem>
                                        {col.key !== 'PRI' && (
                                          <DropdownMenuItem onClick={() => handleDropColumn(table.name, col.name)} className="text-destructive gap-2 cursor-pointer text-xs font-bold" style={{ cursor: 'pointer' }}>
                                            <Trash2 className="w-3.5 h-3.5" />
                                            {t('databaseStudio.structure.actions.dropColumn')}
                                          </DropdownMenuItem>
                                        )}
                                      </DropdownMenuContent>
                                    </DropdownMenu>
                                  </td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      </div>
                    </div>
                  )
                })()
              ) : (
                <div className="flex-1 flex flex-col items-center justify-center p-6 text-center text-muted-foreground gap-4 animate-in fade-in duration-300">
                  <Table className="w-10 h-10 text-muted-foreground/40" />
                  <div className="space-y-1">
                    <h4 className="font-extrabold text-base">{t('databaseStudio.tables.noTableSelected') || 'No table selected'}</h4>
                    <p className="text-xs text-muted-foreground max-w-sm">Select a table from the list to view its columns structure.</p>
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* Create Table Dialog Modal */}
          <Dialog open={designerAction === 'create_table'} onOpenChange={(open: boolean) => !open && setDesignerAction(null)}>
            <DialogContent className="sm:max-w-md bg-card/98 border border-border/80 rounded-xl shadow-2xl backdrop-blur-xl">
              <DialogHeader className="pb-2 border-b border-border/40">
                <DialogTitle className="text-lg font-extrabold flex items-center gap-2 text-foreground/90">
                  <Table className="w-5 h-5 text-primary" />
                  {t('databaseStudio.structure.createTableDialog.submitBtn')}
                </DialogTitle>
                <DialogDescription className="text-xs text-muted-foreground">
                  {t('databaseStudio.structure.createTableDialog.desc')}
                </DialogDescription>
              </DialogHeader>
              <form onSubmit={handleDesignerAction} className="space-y-4 pt-3">
                <div className="space-y-1.5">
                  <Label htmlFor="new_table_name" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.createTableDialog.tableName')}</Label>
                  <Input
                    id="new_table_name"
                    value={newTableName}
                    onChange={(e) => setNewTableName(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ''))}
                    placeholder={t('databaseStudio.structure.createTableDialog.tableNamePlaceholder')}
                    required
                    className="h-10 rounded-xl bg-background/50"
                    autoFocus
                  />
                </div>
                <p className="text-[10px] text-muted-foreground italic leading-relaxed bg-muted/20 p-2.5 rounded-lg border border-border/40">
                  {t('databaseStudio.structure.createTableDialog.autoDesignWarning').split('id').map((part, index) => 
                     index === 1 ? <code key={index}>id</code> : part
                  )}
                </p>
                
                <div className="flex gap-2.5 pt-2">
                  <Button type="submit" disabled={isActionLoading} className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {isActionLoading ? t('common.executing') : t('databaseStudio.structure.createTableDialog.submitBtn')}
                  </Button>
                  <Button type="button" onClick={() => setDesignerAction(null)} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {t('common.cancel')}
                  </Button>
                </div>
              </form>
            </DialogContent>
          </Dialog>

          {/* Add Column Dialog Modal */}
          <Dialog open={designerAction === 'add_column'} onOpenChange={(open: boolean) => !open && setDesignerAction(null)}>
            <DialogContent className="sm:max-w-md bg-card/98 border border-border/80 rounded-xl shadow-2xl backdrop-blur-xl">
              <DialogHeader className="pb-2 border-b border-border/40">
                <DialogTitle className="text-lg font-extrabold flex items-center gap-2 text-foreground/90">
                  <PlusCircle className="w-5 h-5 text-primary" />
                  {t('databaseStudio.structure.addColumn')} — {selectedTable}
                </DialogTitle>
                <DialogDescription className="text-xs text-muted-foreground">
                  {t('databaseStudio.structure.addColumnDialog.desc')}
                </DialogDescription>
              </DialogHeader>
              <form onSubmit={handleDesignerAction} className="space-y-4 pt-3">
                <div className="space-y-1.5">
                  <Label htmlFor="new_col_name" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.createTableDialog.columnName')}</Label>
                  <Input
                    id="new_col_name"
                    value={newColName}
                    onChange={(e) => setNewColName(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ''))}
                    placeholder={t('databaseStudio.structure.addColumnDialog.columnNamePlaceholder')}
                    required
                    className="h-10 rounded-xl bg-background/50"
                    autoFocus
                  />
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1.5">
                    <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.typeHeader')}</Label>
                    <select
                      value={newColType}
                      onChange={(e) => setNewColType(e.target.value)}
                      className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold outline-none focus:border-primary/50 cursor-pointer"
                      style={{ cursor: 'pointer' }}
                    >
                      <option value="varchar">VARCHAR</option>
                      <option value="integer">INTEGER</option>
                      <option value="bigint">BIGINT</option>
                      <option value="text">TEXT</option>
                      <option value="boolean">BOOLEAN</option>
                      <option value="timestamp">TIMESTAMP</option>
                      <option value="decimal">DECIMAL</option>
                    </select>
                  </div>

                  {newColType === 'varchar' && (
                    <div className="space-y-1.5">
                      <Label htmlFor="new_col_len" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.addColumnDialog.lengthLabel')}</Label>
                      <Input
                        id="new_col_len"
                        type="number"
                        value={newColLength}
                        onChange={(e) => {
                          const val = e.target.value;
                          setNewColLength(val === "" ? "" : Number(val));
                        }}
                        placeholder="255"
                        className="h-10 rounded-xl bg-background/50 text-xs"
                      />
                    </div>
                  )}
                </div>

                <div className="flex items-center justify-between border-t pt-3 mt-4">
                  <Label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">{t('databaseStudio.structure.createTableDialog.nullableLabel')}</Label>
                  <button
                    type="button"
                    onClick={() => setNewColNullable(!newColNullable)}
                    className={cn(
                      "relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out",
                      newColNullable ? "bg-primary" : "bg-muted"
                    )}
                    style={{ cursor: 'pointer' }}
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
                  <Button type="submit" disabled={isActionLoading} className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {isActionLoading ? t('common.executing') : t('databaseStudio.structure.addColumnDialog.submitBtn')}
                  </Button>
                  <Button type="button" onClick={() => setDesignerAction(null)} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {t('common.cancel')}
                  </Button>
                </div>
              </form>
            </DialogContent>
          </Dialog>

          {/* Modify Column Dialog Modal */}
          <Dialog open={designerAction === 'modify_column'} onOpenChange={(open: boolean) => !open && setDesignerAction(null)}>
            <DialogContent className="sm:max-w-md bg-card/98 border border-border/80 rounded-xl shadow-2xl backdrop-blur-xl">
              <DialogHeader className="pb-2 border-b border-border/40">
                <DialogTitle className="text-lg font-extrabold flex items-center gap-2 text-foreground/90">
                  <Pencil className="w-5 h-5 text-primary" />
                  {showColModifyPreview ? t('databaseStudio.structure.modifyColumnDialog.previewTitle') : t('databaseStudio.structure.modifyColumnDialog.title')} — {selectedTable}
                </DialogTitle>
                <DialogDescription className="text-xs text-muted-foreground">
                  {showColModifyPreview ? t('databaseStudio.structure.modifyColumnDialog.previewDesc') : t('databaseStudio.structure.modifyColumnDialog.desc')}
                </DialogDescription>
              </DialogHeader>

              {!showColModifyPreview ? (
                <form onSubmit={handleModifyColumnFormSubmit} className="space-y-4 pt-3">
                  <div className="space-y-1.5">
                    <Label htmlFor="edit_col_name" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                      {t('databaseStudio.structure.modifyColumnDialog.columnNameLabel')}
                    </Label>
                    <Input
                      id="edit_col_name"
                      value={editColNewName}
                      onChange={(e) => setEditColNewName(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ''))}
                      placeholder={t('databaseStudio.structure.createTableDialog.columnName')}
                      required
                      className="h-10 rounded-xl bg-background/50"
                      autoFocus
                    />
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <div className="space-y-1.5">
                      <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                        {t('databaseStudio.structure.createTableDialog.dataType')}
                      </Label>
                      <select
                        value={editColType}
                        onChange={(e) => setEditColType(e.target.value)}
                        className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold outline-none focus:border-primary/50 cursor-pointer"
                        style={{ cursor: 'pointer' }}
                      >
                        <option value="varchar">VARCHAR</option>
                        <option value="integer">INTEGER</option>
                        <option value="bigint">BIGINT</option>
                        <option value="text">TEXT</option>
                        <option value="boolean">BOOLEAN</option>
                        <option value="timestamp">TIMESTAMP</option>
                        <option value="decimal">DECIMAL</option>
                      </select>
                    </div>

                    {editColType === 'varchar' && (
                      <div className="space-y-1.5">
                        <Label htmlFor="edit_col_len" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                          {t('databaseStudio.structure.addColumnDialog.lengthLabel')}
                        </Label>
                        <Input
                          id="edit_col_len"
                          type="number"
                          value={editColLength}
                          onChange={(e) => {
                            const val = e.target.value;
                            setEditColLength(val === "" ? "" : Number(val));
                          }}
                          placeholder="255"
                          className="h-10 rounded-xl bg-background/50 text-xs"
                        />
                      </div>
                    )}
                  </div>

                  <div className="space-y-1.5">
                    <Label htmlFor="edit_col_default" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                      {t('databaseStudio.structure.createTableDialog.defaultValue')}
                    </Label>
                    <Input
                      id="edit_col_default"
                      value={editColDefault}
                      onChange={(e) => setEditColDefault(e.target.value)}
                      placeholder="e.g. NULL, 'active', 0"
                      className="h-10 rounded-xl bg-background/50 text-xs font-mono"
                    />
                  </div>

                  <div className="flex items-center justify-between border-t pt-3 mt-4">
                    <Label className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                      {t('databaseStudio.structure.createTableDialog.nullableLabel')}
                    </Label>
                    <button
                      type="button"
                      onClick={() => setEditColNullable(!editColNullable)}
                      className={cn(
                        "relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out",
                        editColNullable ? "bg-primary" : "bg-muted"
                      )}
                      style={{ cursor: 'pointer' }}
                    >
                      <span
                        className={cn(
                          "pointer-events-none inline-block h-4 w-4 transform rounded-full bg-background shadow ring-0 transition duration-200 ease-in-out",
                          editColNullable ? "translate-x-4" : "translate-x-0"
                        )}
                      />
                    </button>
                  </div>

                  <div className="flex gap-2.5 pt-2">
                    <Button type="submit" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                      {t('databaseStudio.structure.modifyColumnDialog.submitBtn')}
                    </Button>
                    <Button type="button" onClick={() => setDesignerAction(null)} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                      {t('common.cancel')}
                    </Button>
                  </div>
                </form>
              ) : (
                <div className="space-y-4 pt-3">
                  <div className="rounded-xl border border-border/80 bg-background/50 overflow-hidden">
                    <div className="flex items-center justify-between bg-muted/30 px-4 py-2 border-b border-border/80">
                      <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">SQL Script</span>
                      <button 
                        type="button" 
                        onClick={() => copyToClipboard(colModifyPreviewSql)} 
                        className="p-1 hover:bg-muted rounded text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                        style={{ cursor: 'pointer' }}
                      >
                        <Copy className="w-3.5 h-3.5" />
                      </button>
                    </div>
                    <textarea
                      readOnly
                      value={colModifyPreviewSql}
                      className="w-full h-44 p-4 font-mono text-xs bg-transparent border-none outline-none resize-none leading-relaxed text-foreground/90 select-all"
                    />
                  </div>

                  <div className="flex gap-2.5 pt-2 border-t border-border/40">
                    <Button onClick={handleCommitModifyColumn} disabled={isActionLoading} className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                      {isActionLoading ? t('common.executing') : t('databaseStudio.structure.modifyColumnDialog.commitBtn')}
                    </Button>
                    <Button onClick={() => setShowColModifyPreview(false)} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                      {t('databaseStudio.structure.modifyColumnDialog.backBtn')}
                    </Button>
                  </div>
                </div>
              )}
            </DialogContent>
          </Dialog>
        </>
      )}

      {activeTab === 'query' && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          {/* Main SQL Area */}
          <div className="lg:col-span-2 space-y-6">
            <Card className="p-6">
              <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b pb-4 mb-4">
                <div>
                  <h3 className="font-extrabold text-base flex items-center gap-2">
                    <Terminal className="w-5 h-5 text-primary" />
                    {t('databaseStudio.query.title')}
                  </h3>
                  <p className="text-muted-foreground text-xs">{t('databaseStudio.query.subtitle')}</p>
                </div>

                <div className="flex items-center gap-3 shrink-0">
                  {!isSuspended && (
                    <select
                      onChange={(e) => {
                        const template = e.target.value
                        if (!template) return
                        
                        const tableName = selectedTable || (schemaData.length > 0 ? schemaData[0].name : 'users')
                        const columns = schemaData.find(t => t.name === tableName)?.columns || []
                        const firstColName = columns.length > 0 ? columns[0].name : 'id'

                        const isPostgres = dbOverview?.engine?.toLowerCase() === 'postgres'
                        const q = isPostgres ? '"' : '`'

                        let sql = ''
                        if (template === 'select') {
                          sql = `SELECT * FROM ${q}${tableName}${q} LIMIT 10;`
                        } else if (template === 'count') {
                          sql = `SELECT COUNT(*) AS total_rows FROM ${q}${tableName}${q};`
                        } else if (template === 'filter') {
                          sql = `SELECT * FROM ${q}${tableName}${q} WHERE ${q}${firstColName}${q} = 1;`
                        } else if (template === 'group') {
                          sql = `SELECT ${q}${firstColName}${q}, COUNT(*) AS count FROM ${q}${tableName}${q} GROUP BY ${q}${firstColName}${q} ORDER BY count DESC;`
                        }
                        
                        if (sql) {
                          setSqlQuery(sql)
                        }
                        e.target.value = '' // Reset selector choice
                      }}
                      className="h-10 px-3.5 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold outline-none focus:border-primary/50 cursor-pointer"
                      style={{ cursor: 'pointer' }}
                    >
                      <option value="">{t('databaseStudio.query.templates.label')}</option>
                      <option value="select">{t('databaseStudio.query.templates.select')}</option>
                      <option value="count">{t('databaseStudio.query.templates.count')}</option>
                      <option value="filter">{t('databaseStudio.query.templates.filter')}</option>
                      <option value="group">{t('databaseStudio.query.templates.group')}</option>
                    </select>
                  )}

                  <Button
                    onClick={handleExecuteSQL}
                    disabled={isActionLoading || isSuspended || !sqlQuery.trim()}
                    className="font-bold shrink-0 gap-1.5 h-10 rounded-xl cursor-pointer"
                    style={{ cursor: 'pointer' }}
                  >
                    <Play className="w-4 h-4 fill-current" />
                    {t('databaseStudio.query.runQuery')}
                  </Button>
                </div>
              </div>

              {isSuspended ? (
                <div className="py-12 text-center text-muted-foreground text-sm font-semibold uppercase tracking-wide">
                  {t('databaseStudio.dashboard.suspendedWarning')}
                </div>
              ) : (
                <div className="space-y-4">
                  <div className="border border-border/85 rounded-xl overflow-hidden focus-within:border-primary/50 transition-all">
                    <textarea
                      value={sqlQuery}
                      onChange={(e) => setSqlQuery(e.target.value)}
                      className="w-full h-44 p-4 font-mono text-xs bg-background/50 border-none outline-none focus:ring-0 leading-relaxed resize-y"
                      placeholder={t('databaseStudio.query.queryPlaceholder')}
                    />
                  </div>

                  {/* SQL Execution Result Grid */}
                  {queryResult && (
                    <div className="border border-border/80 rounded-xl overflow-hidden bg-background/10 animate-in zoom-in-95">
                      <div className="flex justify-between items-center bg-muted/30 px-4 py-3 border-b border-border/80 text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                        <span>{t('databaseStudio.query.outputHeader')}</span>
                        {queryResult.duration && <span>{t('databaseStudio.query.queryDuration')}: {queryResult.duration}</span>}
                      </div>

                      {queryResult.error ? (
                        <div className="p-4 text-xs font-mono text-destructive bg-destructive/5 font-semibold">
                          {t('databaseStudio.query.errorLabel')}: {queryResult.error}
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
                                    {t('databaseStudio.query.noRecords')}
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
                          {t('databaseStudio.query.successMsg', { count: queryResult.rows_affected })}
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
              <div className="flex items-center justify-between border-b pb-2 mb-4">
                <h4 className="font-extrabold text-sm flex items-center gap-2 uppercase tracking-wide">
                  <History className="w-4.5 h-4.5" />
                  {t('databaseStudio.query.history.title')}
                </h4>
                {queryHistory.length > 0 && (
                  <button
                    type="button"
                    onClick={handleClearQueryHistory}
                    className="text-[10px] font-bold uppercase tracking-widest text-rose-500 hover:text-rose-600 cursor-pointer"
                  >
                    {t('databaseStudio.query.history.clearAll')}
                  </button>
                )}
              </div>
              
              {queryHistory.length === 0 ? (
                <div className="py-8 text-center text-xs text-muted-foreground italic font-semibold">
                  {t('databaseStudio.query.history.emptyHistory')}
                </div>
              ) : (
                <div className="space-y-3.5 max-h-[350px] overflow-y-auto">
                  {queryHistory.map((q, idx) => (
                    <button
                      key={idx}
                      onClick={() => setSqlQuery(q)}
                      className="w-full text-left p-3 border rounded-xl hover:border-primary/30 font-mono text-[10px] text-muted-foreground hover:text-foreground hover:bg-muted/5 transition-all text-ellipsis overflow-hidden whitespace-nowrap block cursor-pointer"
                      style={{ cursor: 'pointer' }}
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
                  <h3 className="font-extrabold text-base">{t('databaseStudio.backups.title')}</h3>
                  <p className="text-muted-foreground text-xs">{t('databaseStudio.backups.desc')}</p>
                </div>
                
                <Button
                  size="sm"
                  onClick={handleCreateBackup}
                  disabled={isActionLoading || isSuspended}
                  className="font-bold shrink-0 gap-1.5 h-10 rounded-xl cursor-pointer"
                  style={{ cursor: 'pointer' }}
                >
                  <PlusCircle className="w-4 h-4" />
                  {t('databaseStudio.backups.captureBtn')}
                </Button>
              </div>

              {isSuspended ? (
                <div className="py-12 text-center text-muted-foreground text-sm font-semibold uppercase tracking-wide">
                  {t('databaseStudio.dashboard.suspendedWarning')}
                </div>
              ) : backups.length === 0 ? (
                <div className="py-16 text-center border border-dashed rounded-xl flex flex-col items-center justify-center gap-4 bg-muted/5 animate-in fade-in duration-300">
                  <DatabaseZap className="w-10 h-10 text-muted-foreground/60" />
                  <div className="space-y-1">
                    <h4 className="font-extrabold text-base">{t('databaseStudio.backups.empty')}</h4>
                    <p className="text-xs text-muted-foreground max-w-sm">{t('databaseStudio.backups.emptyDesc')}</p>
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
                          <span>{t('databaseStudio.backups.capturedAt', { time: new Date(b.created_at).toLocaleString() })}</span>
                          <span className="w-1 h-1 bg-muted-foreground rounded-full" />
                          <span className={cn(
                            "px-2 py-0.5 rounded text-[8px] font-extrabold uppercase border",
                            b.status === 'completed' 
                              ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/20" 
                              : b.status === 'pending'
                              ? "bg-amber-500/10 text-amber-500 border-amber-500/20 animate-pulse"
                              : "bg-destructive/10 text-destructive border-destructive/20"
                          )}>
                            {t('status.' + b.status)}
                          </span>
                        </div>
                      </div>

                      <div className="flex items-center gap-2.5 shrink-0 justify-end">
                        <Button
                          variant="outline"
                          size="xs"
                          onClick={() => handleDownloadBackup(b.id, b.name)}
                          disabled={isActionLoading || b.status !== 'completed'}
                          className="h-8 text-xs font-bold gap-1 rounded-lg hover:border-primary/20 hover:text-primary transition-colors cursor-pointer"
                          style={{ cursor: 'pointer' }}
                        >
                          <Download className="w-3.5 h-3.5" />
                          {t('databaseStudio.backups.actions.download')}
                        </Button>
                        <Button
                          variant="outline"
                          size="xs"
                          onClick={() => handleRestoreBackup(b.id)}
                          disabled={isActionLoading || b.status !== 'completed'}
                          className="h-8 text-xs font-bold gap-1 rounded-lg hover:border-primary/20 hover:text-primary transition-colors cursor-pointer"
                          style={{ cursor: 'pointer' }}
                        >
                          <RefreshCw className="w-3.5 h-3.5" />
                          {t('databaseStudio.backups.actions.restore')}
                        </Button>
                        <Button
                          variant="outline"
                          size="xs"
                          onClick={() => handleDeleteBackup(b.id)}
                          disabled={isActionLoading}
                          className="h-8 text-xs font-bold gap-1 rounded-lg hover:bg-destructive/10 hover:text-destructive hover:border-destructive/30 transition-colors cursor-pointer"
                          style={{ cursor: 'pointer' }}
                        >
                          <Trash2 className="w-3.5 h-3.5 cursor-pointer" style={{ cursor: 'pointer' }} />
                          {t('databaseStudio.backups.actions.prune')}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </Card>
          </div>

          {/* Backup Retention Card */}
          <div className="space-y-6">
            <Card className="p-5 border-muted-foreground/10 bg-muted/5">
              <h4 className="font-bold text-xs mb-3 flex items-center gap-2 text-foreground/80 uppercase tracking-wider border-b pb-2">
                <Shield className="w-4 h-4" />
                {t('databaseStudio.backups.retentionTitle')}
              </h4>
              <p className="text-xs text-muted-foreground leading-relaxed">
                {t('databaseStudio.backups.retentionDesc')}
              </p>
              <div className="flex justify-between items-center text-xs mt-4 pt-3 border-t border-muted-foreground/10">
                <span className="font-semibold text-muted-foreground">{t('databaseStudio.backups.catalogCapacity')}</span>
                <span className="font-mono font-bold text-foreground">
                  {backups.filter(b => b.status === 'completed').length} / 5 {t('databaseStudio.backups.snapshotsLabel')}
                </span>
              </div>
            </Card>
          </div>
        </div>
      )}

      {/* Structured Confirmation Modal Overlay */}
      <ConfirmationModal
        isOpen={confirmModal.isOpen}
        onClose={closeConfirmation}
        onConfirm={confirmModal.onConfirm}
        title={confirmModal.title}
        message={confirmModal.message}
        type={confirmModal.type}
        confirmText={confirmModal.confirmText || t('common.confirm')}
        cancelText={confirmModal.cancelText || t('common.cancel')}
      />
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
