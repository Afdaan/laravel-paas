import React, { useState, useEffect, useCallback } from 'react'
import { useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { databaseAPI } from '../../services/api'
import { DatabaseBackup } from '../../types'
import { siMysql, siPostgresql } from 'simple-icons'
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
  Activity,
  HardDrive,
  Table,
  PlusCircle,
  AlertTriangle,
  DatabaseZap,
  Search,
  Download,
  Pencil,
  Link,
  MoreHorizontal,
  Info
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

function DatabaseEngineIcon({ engine, className }: { engine?: string; className?: string }) {
  const norm = (engine || '').toLowerCase().trim();
  let icon = siMysql;
  if (norm.includes('post') || norm.includes('pg')) {
    icon = siPostgresql;
  }

  return (
    <svg 
      viewBox="0 0 24 24" 
      aria-hidden="true" 
      className={cn('w-5 h-5 shrink-0', className)} 
      xmlns="http://www.w3.org/2000/svg"
    >
      <path fill={`#${icon.hex}`} d={icon.path} />
    </svg>
  );
}

const getEngineDisplayName = (engine?: string) => {
  const norm = (engine || '').toLowerCase().trim();
  if (norm.includes('post') || norm.includes('pg')) {
    return 'PostgreSQL';
  }
  return 'MySQL';
};

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

const adjustDatetimeForDatabase = (inputValue: string): string => {
  if (!inputValue) return inputValue

  try {
    const match = inputValue.match(/^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2})(?::(\d{2}))?/)
    if (!match) return inputValue

    const [_, year, month, day, hours, minutes, secondsStr] = match
    const seconds = secondsStr ? Number(secondsStr) : 0
    
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${year}-${pad(Number(month))}-${pad(Number(day))} ${pad(Number(hours))}:${pad(Number(minutes))}:${pad(seconds)}`
  } catch (e) {
    return inputValue
  }
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

    // Match YYYY-MM-DD HH:mm:ss (with optional fractional seconds and timezone) timezone-agnostically
    const regex = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(Z|[+-]\d{2}(?::?\d{2})?)?$/i
    const match = strVal.match(regex)
    if (match) {
      const [_, year, month, day, hours, minutes] = match
      return `${year}-${month}-${day}T${hours}:${minutes}`
    }

    // Match YYYY-MM-DD HH:mm (with optional timezone) timezone-agnostically
    const regexShort = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2})(?:(Z|[+-]\d{2}(?::?\d{2})?))?$/i
    const matchShort = strVal.match(regexShort)
    if (matchShort) {
      const [_, year, month, day, hours, minutes] = matchShort
      return `${year}-${month}-${day}T${hours}:${minutes}`
    }

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

const formatHumanDatetime = (val: any) => {
  if (val === null || val === undefined) return ''
  try {
    const strVal = String(val).trim()
    if (!strVal) return ''

    // Parse timezone-agnostically first to prevent browser timezone shifts
    const regex = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(Z|[+-]\d{2}(?::?\d{2})?)?$/i
    const match = strVal.match(regex)
    let d: Date
    if (match) {
      const [_, year, month, day, hours, minutes, seconds] = match
      d = new Date(Number(year), Number(month) - 1, Number(day), Number(hours), Number(minutes), Number(seconds))
    } else {
      const regexShort = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2})(?:(Z|[+-]\d{2}(?::?\d{2})?))?$/i
      const matchShort = strVal.match(regexShort)
      if (matchShort) {
        const [_, year, month, day, hours, minutes] = matchShort
        d = new Date(Number(year), Number(month) - 1, Number(day), Number(hours), Number(minutes), 0)
      } else {
        const parsedStr = strVal.includes(' ') && !strVal.includes('T') ? strVal.replace(' ', 'T') : strVal
        d = new Date(parsedStr)
      }
    }

    if (isNaN(d.getTime())) return strVal

    const year = d.getFullYear()
    const monthNames = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]
    const month = monthNames[d.getMonth()]
    const day = String(d.getDate()).padStart(2, '0')
    const hours = String(d.getHours()).padStart(2, '0')
    const minutes = String(d.getMinutes()).padStart(2, '0')
    const seconds = String(d.getSeconds()).padStart(2, '0')
    
    return `${day} ${month} ${year}, ${hours}:${minutes}:${seconds}`
  } catch (e) {
    return String(val)
  }
}

const formatHumanDate = (val: any) => {
  if (val === null || val === undefined) return ''
  try {
    const strVal = String(val).trim()
    if (!strVal) return ''

    // Parse timezone-agnostically first to prevent browser timezone shifts
    const regex = /^(\d{4})-(\d{2})-(\d{2})/i
    const match = strVal.match(regex)
    let d: Date
    if (match) {
      const [_, year, month, day] = match
      d = new Date(Number(year), Number(month) - 1, Number(day), 0, 0, 0)
    } else {
      const parsedStr = strVal.includes(' ') && !strVal.includes('T') ? strVal.replace(' ', 'T') : strVal
      d = new Date(parsedStr)
    }

    if (isNaN(d.getTime())) return strVal

    const year = d.getFullYear()
    const monthNames = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]
    const month = monthNames[d.getMonth()]
    const day = String(d.getDate()).padStart(2, '0')
    
    return `${day} ${month} ${year}`
  } catch (e) {
    return String(val)
  }
}

const formatCellValue = (val: any): React.ReactNode => {
  if (val === null || val === undefined) {
    return <span className="text-muted-foreground/30 italic">NULL</span>
  }
  
  const strVal = String(val).trim()
  if (!strVal) return strVal

  // Check if it matches ISO datetime or DB space-separated datetime
  const datetimeRegex = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(Z|[+-]\d{2}(?::?\d{2})?)?$/i
  if (datetimeRegex.test(strVal)) {
    return formatHumanDatetime(strVal)
  }

  // Check if it matches YYYY-MM-DD date-only
  const dateRegex = /^(\d{4})-(\d{2})-(\d{2})$/
  if (dateRegex.test(strVal)) {
    return formatHumanDate(strVal)
  }

  return strVal
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
  const [editColPk, setEditColPk] = useState(false)
  const [editColUnique, setEditColUnique] = useState(false)
  const [editColComment, setEditColComment] = useState('')
  const [editColFk, setEditColFk] = useState(false)
  const [editColFkTargetTable, setEditColFkTargetTable] = useState('')
  const [editColFkTargetColumn, setEditColFkTargetColumn] = useState('')
  const [editColFkOnDelete, setEditColFkOnDelete] = useState('CASCADE')
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
  const [newColDefault, setNewColDefault] = useState('')
  const [newColPk, setNewColPk] = useState(false)
  const [newColUnique, setNewColUnique] = useState(false)
  const [newColComment, setNewColComment] = useState('')
  const [newColFk, setNewColFk] = useState(false)
  const [newColFkTargetTable, setNewColFkTargetTable] = useState('')
  const [newColFkTargetColumn, setNewColFkTargetColumn] = useState('')
  const [newColFkOnDelete, setNewColFkOnDelete] = useState('CASCADE')
  
  const [indexName] = useState('')
  const [indexCols] = useState<string[]>([])

  const resetAddColumnForm = () => {
    setNewColName('')
    setNewColType('varchar')
    setNewColLength(255)
    setNewColNullable(true)
    setNewColDefault('')
    setNewColPk(false)
    setNewColUnique(false)
    setNewColComment('')
    setNewColFk(false)
    setNewColFkTargetTable('')
    setNewColFkTargetColumn('')
    setNewColFkOnDelete('CASCADE')
    setDesignerAction(null)
  }

  const resetModifyColumnForm = () => {
    setEditingCol(null)
    setEditColNewName('')
    setEditColType('varchar')
    setEditColLength(255)
    setEditColNullable(true)
    setEditColDefault('')
    setEditColPk(false)
    setEditColUnique(false)
    setEditColComment('')
    setEditColFk(false)
    setEditColFkTargetTable('')
    setEditColFkTargetColumn('')
    setEditColFkOnDelete('CASCADE')
    setShowColModifyPreview(false)
    setColModifyPreviewSql('')
    setDesignerAction(null)
  }

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

        let val = insertFormData[col.name]
        if (val === undefined || val === '') return

        fields.push(`${q}${col.name}${q}`)

        const typeLower = col.type.toLowerCase()
        if (typeLower.includes('timestamp') || typeLower.includes('datetime')) {
          val = adjustDatetimeForDatabase(val)
        }

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

      const typeLower = col.type.toLowerCase()
      let val = editFormData[col.name]
      const escapedCol = `${q}${col.name}${q}`

      if (val === undefined || val === '') {
        if (col.nullable) {
          setClauses.push(`${escapedCol} = NULL`)
        } else {
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

      if (typeLower.includes('timestamp') || typeLower.includes('datetime')) {
        val = adjustDatetimeForDatabase(val)
      }

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

        const typeLower = col.type.toLowerCase()
        let val = editFormData[col.name]
        if (val === undefined || val === '') {
          if (col.nullable) {
            updates[col.name] = null
          } else {
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

        if (typeLower.includes('timestamp') || typeLower.includes('datetime')) {
          val = adjustDatetimeForDatabase(val)
        }

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
    
    const table = schemaData.find(t => t.name === tableName)
    const tableFks = table?.foreign_keys || []
    const fk = tableFks.find((f: any) => f.column_name === col.name)
    
    const parsed = parseDbType(col.type)
    setEditColType(parsed.type)
    setEditColLength(parsed.length)
    setEditColNullable(col.nullable)
    setEditColDefault(col.default !== null ? String(col.default) : '')
    setEditColPk(col.key === 'PRI')
    setEditColUnique(col.key === 'UNI')
    setEditColComment(col.comment || '')
    setEditColFk(!!fk)
    setEditColFkTargetTable(fk ? fk.target_table : '')
    setEditColFkTargetColumn(fk ? fk.target_column : '')
    setEditColFkOnDelete(fk ? (fk.on_delete || 'CASCADE') : 'CASCADE')
    
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
      
      const oldUqName = `uq_${selectedTable}_${editingCol.name}`
      sqls.push(`-- 4. Drop Old Unique Constraint (if any)\nALTER TABLE ${escapedTable} DROP CONSTRAINT IF EXISTS ${q}${oldUqName}${q};`)
      
      const oldFkName = `fk_${selectedTable}_${editingCol.name}`
      sqls.push(`-- 5. Drop Old Foreign Key Constraint (if any)\nALTER TABLE ${escapedTable} DROP CONSTRAINT IF EXISTS ${q}${oldFkName}${q};`)

      let colNameAfterRename = editingCol.name
      if (editColNewName && editColNewName !== editingCol.name) {
        const escapedNewCol = `${q}${editColNewName}${q}`
        sqls.push(`-- 6. Rename Column\nALTER TABLE ${escapedTable} RENAME COLUMN ${escapedCol} TO ${escapedNewCol};`)
        colNameAfterRename = editColNewName
      }
      const escapedColAfterRename = `${q}${colNameAfterRename}${q}`

      if (editColComment !== undefined) {
        if (editColComment === '') {
          sqls.push(`-- 7. Remove Column Comment\nCOMMENT ON COLUMN ${escapedTable}.${escapedColAfterRename} IS NULL;`)
        } else {
          sqls.push(`-- 7. Set Column Comment\nCOMMENT ON COLUMN ${escapedTable}.${escapedColAfterRename} IS '${editColComment.replace(/'/g, "''")}';`)
        }
      }

      const pkeyName = `${selectedTable}_pkey`
      sqls.push(`-- 8. Drop Primary Key Constraint (if any)\nALTER TABLE ${escapedTable} DROP CONSTRAINT IF EXISTS ${q}${pkeyName}${q};`)
      if (editColPk) {
        sqls.push(`-- 9. Add Primary Key Constraint\nALTER TABLE ${escapedTable} ADD PRIMARY KEY (${escapedColAfterRename});`)
      }

      if (editColUnique) {
        const newUqName = `uq_${selectedTable}_${colNameAfterRename}`
        sqls.push(`-- 10. Add Unique Constraint\nALTER TABLE ${escapedTable} ADD CONSTRAINT ${q}${newUqName}${q} UNIQUE (${escapedColAfterRename});`)
      }

      if (editColFk && editColFkTargetTable && editColFkTargetColumn) {
        const newFkName = `fk_${selectedTable}_${colNameAfterRename}`
        sqls.push(`-- 11. Add Foreign Key Constraint\nALTER TABLE ${escapedTable} ADD CONSTRAINT ${q}${newFkName}${q} FOREIGN KEY (${escapedColAfterRename}) REFERENCES ${q}${editColFkTargetTable}${q} (${q}${editColFkTargetColumn}${q}) ON DELETE ${editColFkOnDelete.toUpperCase()};`)
      }

      return sqls.join('\n\n')
    } else {
      const sqls: string[] = []
      const commentSuffix = editColComment ? ` COMMENT '${editColComment.replace(/'/g, "''")}'` : ''
      sqls.push(`-- 1. Modify Column Structure\nALTER TABLE ${escapedTable} MODIFY COLUMN ${escapedCol} ${dbType} ${nullability}${defaultClause}${commentSuffix};`)

      const oldUqName = `uq_${selectedTable}_${editingCol.name}`
      sqls.push(`-- 2. Drop Old Unique Index (if any)\nALTER TABLE ${escapedTable} DROP INDEX ${q}${oldUqName}${q};`)

      const oldFkName = `fk_${selectedTable}_${editingCol.name}`
      sqls.push(`-- 3. Drop Old Foreign Key Constraint (if any)\nALTER TABLE ${escapedTable} DROP FOREIGN KEY ${q}${oldFkName}${q};`)

      let colNameAfterRename = editingCol.name
      if (editColNewName && editColNewName !== editingCol.name) {
        const escapedNewCol = `${q}${editColNewName}${q}`
        sqls.push(`-- 4. Rename Column\nALTER TABLE ${escapedTable} RENAME COLUMN ${escapedCol} TO ${escapedNewCol};`)
        colNameAfterRename = editColNewName
      }
      const escapedColAfterRename = `${q}${colNameAfterRename}${q}`

      sqls.push(`-- 5. Drop Primary Key (if any)\nALTER TABLE ${escapedTable} DROP PRIMARY KEY;`)
      if (editColPk) {
        sqls.push(`-- 6. Add Primary Key\nALTER TABLE ${escapedTable} ADD PRIMARY KEY (${escapedColAfterRename});`)
      }

      if (editColUnique) {
        const newUqName = `uq_${selectedTable}_${colNameAfterRename}`
        sqls.push(`-- 7. Add Unique Constraint\nALTER TABLE ${escapedTable} ADD CONSTRAINT ${q}${newUqName}${q} UNIQUE (${escapedColAfterRename});`)
      }

      if (editColFk && editColFkTargetTable && editColFkTargetColumn) {
        const newFkName = `fk_${selectedTable}_${colNameAfterRename}`
        sqls.push(`-- 8. Add Foreign Key Constraint\nALTER TABLE ${escapedTable} ADD CONSTRAINT ${q}${newFkName}${q} FOREIGN KEY (${escapedColAfterRename}) REFERENCES ${q}${editColFkTargetTable}${q} (${q}${editColFkTargetColumn}${q}) ON DELETE ${editColFkOnDelete.toUpperCase()};`)
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
        default_value: editColDefault === "" ? null : editColDefault,
        primary_key: editColPk,
        unique: editColUnique,
        comment: editColComment === "" ? null : editColComment,
        foreign_key: editColFk,
        fk_table: editColFkTargetTable,
        fk_column: editColFkTargetColumn,
        fk_on_delete: editColFkOnDelete
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
        nullable: newColNullable,
        default_value: newColDefault === "" ? null : newColDefault,
        primary_key: newColPk,
        unique: newColUnique,
        comment: newColComment === "" ? null : newColComment,
        foreign_key: newColFk,
        fk_table: newColFkTargetTable,
        fk_column: newColFkTargetColumn,
        fk_on_delete: newColFkOnDelete
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
              <Card className="p-5 flex flex-col gap-2 hover:border-primary/20 transition-all duration-300">
                <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.engine')}</span>
                <span className="text-sm font-bold text-foreground flex items-center gap-2 h-6">
                  <DatabaseEngineIcon engine={dbOverview?.engine} />
                  {getEngineDisplayName(dbOverview?.engine)}
                </span>
              </Card>

              <Card className="p-5 flex flex-col gap-2 hover:border-primary/20 transition-all duration-300">
                <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.version')}</span>
                <span className="text-xs font-mono font-bold text-foreground truncate flex items-center h-6" title={dbOverview?.version}>
                  {dbOverview?.version || 'Unknown'}
                </span>
              </Card>

              <Card className="p-5 flex flex-col gap-2 hover:border-primary/20 transition-all duration-300">
                <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.diskUsed')}</span>
                <span className="text-sm font-mono font-bold text-foreground flex items-center gap-2 h-6">
                  <HardDrive className="w-4 h-4 text-primary shrink-0" />
                  {dbOverview?.size || '0 KB'}
                </span>
              </Card>

              <Card className="p-5 flex flex-col gap-2 hover:border-primary/20 transition-all duration-300">
                <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.tables')}</span>
                <span className="text-sm font-mono font-bold text-foreground flex items-center gap-2 h-6">
                  <Table className="w-4 h-4 text-primary shrink-0" />
                  {dbOverview?.table_count || 0}
                </span>
              </Card>
            </div>

            {/* Connection Metrics */}
            {metrics && (() => {
              const connectionRatio = metrics.active_connections / 15;
              const connectionColor = connectionRatio > 0.8 
                ? 'bg-destructive' 
                : connectionRatio > 0.6 
                  ? 'bg-amber-500' 
                  : 'bg-primary';

              const storageRatio = metrics.size_kb / 1048576;
              const storageColor = storageRatio > 0.8 
                ? 'bg-destructive' 
                : storageRatio > 0.6 
                  ? 'bg-amber-500' 
                  : 'bg-primary';

              return (
                <Card className="p-6">
                  <div className="flex items-center justify-between mb-5">
                    <h3 className="font-extrabold text-xs uppercase tracking-wider text-muted-foreground flex items-center gap-2">
                      {t('databaseStudio.dashboard.metrics.resourceUsageTitle')}
                    </h3>
                    <div className="flex items-center gap-1.5 bg-emerald-500/10 border border-emerald-500/20 px-2.5 py-0.5 rounded-full">
                      <span className="relative flex h-1.5 w-1.5">
                        <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                        <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-emerald-500"></span>
                      </span>
                      <span className="text-[9px] font-extrabold text-emerald-600 uppercase tracking-wider">
                        {t('databaseStudio.dashboard.metrics.realtime') || 'Real-time'}
                      </span>
                    </div>
                  </div>
                  
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
                    {/* Active Connections Widget */}
                    <div className="border border-border/50 bg-muted/10 dark:bg-muted/5 p-4 rounded-xl space-y-3 hover:border-primary/10 transition-colors">
                      <div className="flex justify-between items-center">
                        <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">
                          {t('databaseStudio.dashboard.metrics.activeConnections')}
                        </span>
                        <span className="text-xs font-mono font-bold text-foreground">
                          {metrics.active_connections} <span className="text-muted-foreground/60">/ 15</span>
                        </span>
                      </div>
                      <div className="h-1.5 w-full bg-muted rounded-full overflow-hidden">
                        <div 
                           className={cn("h-full transition-all duration-500", connectionColor)} 
                           style={{ width: `${Math.min(connectionRatio * 100, 100)}%` }}
                        />
                      </div>
                      <p className="text-[10px] text-muted-foreground/60 leading-normal flex items-start gap-1">
                        <Info size={11} className="mt-0.5 shrink-0 text-muted-foreground/40" />
                        <span>{t('databaseStudio.dashboard.metrics.connectionsLimitDesc')}</span>
                      </p>
                    </div>

                    {/* Storage Usage Widget */}
                    <div className="border border-border/50 bg-muted/10 dark:bg-muted/5 p-4 rounded-xl space-y-3 hover:border-primary/10 transition-colors">
                      <div className="flex justify-between items-center">
                        <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">
                          {t('databaseStudio.dashboard.metrics.storageUsage')}
                        </span>
                        <span className="text-xs font-mono font-bold text-foreground">
                          {dbOverview?.size || '0 KB'} <span className="text-muted-foreground/60">/ 1 GB</span>
                        </span>
                      </div>
                      <div className="h-1.5 w-full bg-muted rounded-full overflow-hidden">
                        <div 
                           className={cn("h-full transition-all duration-500", storageColor)} 
                           style={{ width: `${Math.min(storageRatio * 100, 100)}%` }}
                        />
                      </div>
                      <p className="text-[10px] text-muted-foreground/60 leading-normal flex items-start gap-1">
                        <Info size={11} className="mt-0.5 shrink-0 text-muted-foreground/40" />
                        <span>{t('databaseStudio.dashboard.metrics.storageLimitDesc')}</span>
                      </p>
                    </div>
                  </div>
                </Card>
              );
            })()}

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
                  <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.tables.stats.size')}</span>
                  <span className="text-lg font-extrabold text-foreground mt-0.5">
                    {schemaData.find(t => t.name === selectedTable)?.size || '0.00 KB'}
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
                      <tr className="bg-muted border-b border-border/80 text-[10px] font-bold uppercase tracking-widest text-muted-foreground sticky top-0 z-10">
                        <th className="py-3.5 px-4 w-12 text-center bg-muted">{t('databaseStudio.tables.actionHeader')}</th>
                        {tableData.columns.map((col: string) => (
                          <th key={col} className="py-3.5 px-4 font-mono font-semibold bg-muted">{col}</th>
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
                                  <DropdownMenuContent align="center" className="w-32 bg-card border border-border/80 rounded-xl shadow-xl">
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
                                  {formatCellValue(row[col])}
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
          {showInsertModal && (
            <Dialog open={showInsertModal} onOpenChange={(open: boolean) => !open && setShowInsertModal(false)}>
              <DialogContent className="sm:max-w-md bg-card border border-border/80 rounded-xl shadow-2xl">
                <DialogHeader className="pb-2 border-b border-border/40">
                  <DialogTitle className="text-lg font-extrabold flex items-center gap-2 text-foreground/90">
                    <PlusCircle className="w-5 h-5 text-primary" />
                    {t('databaseStudio.tables.insertModal.title')}
                  </DialogTitle>
                  <DialogDescription className="text-xs text-muted-foreground">
                    {t('databaseStudio.tables.insertModal.desc')}
                  </DialogDescription>
                </DialogHeader>
                <form onSubmit={handleInsertRowSubmit} className="space-y-4 pt-3">
                  <div className="space-y-3 max-h-[50vh] overflow-y-auto pr-1">
                    {(tableData?.columns || []).map((col: any) => {
                      const isPK = col.key === 'PRI' || col.extra?.toLowerCase().includes('auto_increment')
                      const isNullable = col.nullable === 'YES' || col.null === 'YES'
                      const typeLower = (col.type || '').toLowerCase()
                      
                      return (
                        <div key={col.name} className="space-y-1.5">
                          <div className="flex items-center justify-between">
                            <Label htmlFor={`insert_${col.name}`} className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground flex items-center gap-1">
                              {col.name}
                              <span className="font-mono text-[9px] text-muted-foreground/50 lowercase">({col.type})</span>
                              {!isNullable && !isPK && <span className="text-red-500 font-bold">*</span>}
                            </Label>
                            
                            {isPK ? (
                              <span className="text-[9px] font-bold uppercase text-amber-500 bg-amber-500/10 px-1.5 py-0.5 rounded border border-amber-500/20">
                                Primary Key
                              </span>
                            ) : isNullable ? (
                              <span className="text-[9px] font-bold uppercase text-muted-foreground/60 bg-muted/10 px-1.5 py-0.5 rounded border border-border/40">
                                Optional
                              </span>
                            ) : undefined}
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
                            <Select
                              value={String(insertFormData[col.name] ?? '')}
                              onValueChange={(val) => setInsertFormData(prev => ({ ...prev, [col.name]: val }))}
                            >
                              <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold text-left justify-between">
                                <SelectValue placeholder={t('databaseStudio.tables.booleanSelect') || undefined} />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem value="true" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">
                                  {t('databaseStudio.tables.booleanTrue')}
                                </SelectItem>
                                <SelectItem value="false" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">
                                  {t('databaseStudio.tables.booleanFalse')}
                                </SelectItem>
                              </SelectContent>
                            </Select>
                          ) : typeLower.includes('timestamp') || typeLower.includes('datetime') || typeLower.includes('date') ? (
                            <input
                              id={`insert_${col.name}`}
                              key={selectedTable + '_' + col.name}
                              type={typeLower.includes('date') && !typeLower.includes('time') ? "date" : "datetime-local"}
                              defaultValue={insertFormData[col.name] || ''}
                              onChange={(e) => {
                                const val = e.target.value
                                const isBadInput = e.target.validity?.badInput
                                if (val === '' && isBadInput) return
                                setInsertFormData(prev => ({ ...prev, [col.name]: val }))
                              }}
                              required={!isNullable}
                              className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold outline-none focus:border-primary/50"
                            />
                          ) : (
                            <Input
                              id={`insert_${col.name}`}
                              value={insertFormData[col.name] || ''}
                              onChange={(e) => setInsertFormData(prev => ({ ...prev, [col.name]: e.target.value }))}
                              placeholder={isNullable ? "NULL" : "Enter value..."}
                              required={!isNullable}
                              className="h-10 rounded-xl bg-background/50 text-xs"
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
          )}

          {/* Visual Edit Row Modal */}
          {showEditModal && (
            <Dialog open={showEditModal} onOpenChange={(open: boolean) => !open && setShowEditModal(false)}>
            <DialogContent className="sm:max-w-md bg-card border border-border/80 rounded-xl shadow-2xl">
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
                            <Select
                              value={String(editFormData[col.name] ?? '')}
                              onValueChange={(val) => setEditFormData(prev => ({ ...prev, [col.name]: val }))}
                            >
                              <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold text-left justify-between">
                                <SelectValue placeholder={t('databaseStudio.tables.booleanSelect') || undefined} />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem value="true" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">
                                  {t('databaseStudio.tables.booleanTrue')}
                                </SelectItem>
                                <SelectItem value="false" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">
                                  {t('databaseStudio.tables.booleanFalse')}
                                </SelectItem>
                              </SelectContent>
                            </Select>
                          ) : typeLower.includes('timestamp') || typeLower.includes('datetime') || typeLower.includes('date') ? (
                            <input
                              id={`edit_${col.name}`}
                              key={editingRow ? `${editingRow[pkColumn]}_${col.name}` : col.name}
                              type={typeLower.includes('date') && !typeLower.includes('time') ? "date" : "datetime-local"}
                              defaultValue={editFormData[col.name] || ''}
                              onChange={(e) => {
                                const val = e.target.value
                                const isBadInput = e.target.validity?.badInput
                                if (val === '' && isBadInput) return
                                setEditFormData(prev => ({ ...prev, [col.name]: val }))
                              }}
                              required={!isNullable}
                              className="flex h-10 w-full rounded-xl border border-border/70 bg-background/50 px-3 py-2 text-xs font-mono file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground outline-none focus:border-primary/50 disabled:cursor-not-allowed disabled:opacity-50"
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
          )}
        </div>
      )}      {activeTab === 'structure' && (
        <>
          <div className="border border-border/80 shadow-2xl overflow-hidden flex flex-col md:flex-row h-[600px] bg-card rounded-xl animate-in fade-in duration-300">
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
                              <tr className="border-b border-border/40 bg-muted text-[10px] font-bold uppercase tracking-wider text-muted-foreground sticky top-0 z-10">
                                <th className="py-3 px-4">{t('databaseStudio.structure.nameHeader')}</th>
                                <th className="py-3 px-4">{t('databaseStudio.structure.typeHeader')}</th>
                                <th className="py-3 px-4 text-center">{t('databaseStudio.structure.createTableDialog.nullableLabel')}</th>
                                <th className="py-3 px-4 text-center">{t('databaseStudio.structure.constraintsHeader') || t('databaseStudio.structure.keyHeader')}</th>
                                <th className="py-3 px-4">{t('databaseStudio.structure.defaultHeader')}</th>
                                <th className="py-3 px-4 text-right">{t('databaseStudio.tables.actionHeader')}</th>
                              </tr>
                            </thead>
                            <tbody>
                              {table.columns.map((col: any) => {
                                const tableFks = table.foreign_keys || []
                                const fk = tableFks.find((f: any) => f.column_name === col.name)
                                const isPK = col.key === 'PRI'
                                const isUQ = col.key === 'UNI'
                                const isIDX = col.key === 'MUL'

                                return (
                                  <tr key={col.name} className="border-b border-border/15 hover:bg-muted/5 transition-colors">
                                    <td className="py-3 px-4 font-mono font-semibold text-foreground/90 flex items-center gap-1.5" title={col.comment || undefined}>
                                      {isPK && <span title="Primary Key"><Key className="w-3.5 h-3.5 text-purple-500" /></span>}
                                      {fk && <span title={`Foreign Key: -> ${fk.target_table}(${fk.target_column})`}><Link className="w-3.5 h-3.5 text-blue-500" /></span>}
                                      <span>{col.name}</span>
                                      {col.comment && (
                                        <span className="text-[10px] text-muted-foreground/60 italic font-normal max-w-[150px] truncate ml-1" title={col.comment}>
                                          ({col.comment})
                                        </span>
                                      )}
                                    </td>
                                    <td className="py-3 px-4 font-mono text-primary/80">
                                      <div className="flex items-center gap-1.5">
                                        <span 
                                          className={cn("w-1.5 h-1.5 rounded-full shrink-0", col.nullable ? "bg-emerald-500" : "bg-amber-500")} 
                                          title={col.nullable ? 'Nullable' : 'Not Null'} 
                                        />
                                        <span>{col.type}</span>
                                      </div>
                                    </td>
                                    <td className="py-3 px-4 text-center">
                                      <span className={cn(
                                        "px-2 py-0.5 rounded text-[10px] font-bold border",
                                        col.nullable 
                                          ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/20" 
                                          : "bg-amber-500/10 text-amber-500 border-amber-500/20"
                                      )}>
                                        {col.nullable ? 'NULLABLE' : 'NOT NULL'}
                                      </span>
                                    </td>
                                    <td className="py-3 px-4">
                                      <div className="flex flex-wrap justify-center gap-1">
                                        {isPK && (
                                          <span 
                                            className="px-2 py-0.5 rounded bg-purple-500/10 text-purple-600 border border-purple-500/20 text-[10px] font-extrabold uppercase tracking-wide flex items-center gap-1 cursor-help"
                                            title={t('databaseStudio.structure.tooltips.primaryKey') || undefined}
                                          >
                                            <Key className="w-2.5 h-2.5" /> PK
                                          </span>
                                        )}
                                        {fk && (
                                          <span 
                                            className="px-2 py-0.5 rounded bg-blue-500/10 text-blue-600 border border-blue-500/20 text-[10px] font-bold uppercase tracking-wide flex items-center gap-1 cursor-help"
                                            title={t('databaseStudio.structure.tooltips.foreignKey', { table: fk.target_table, column: fk.target_column }) || undefined}
                                          >
                                            <Link className="w-2.5 h-2.5" /> FK → {fk.target_table}({fk.target_column})
                                          </span>
                                        )}
                                        {isUQ && (
                                          <span 
                                            className="px-2 py-0.5 rounded bg-orange-500/10 text-orange-600 border border-orange-500/20 text-[10px] font-extrabold uppercase tracking-wide flex items-center gap-1 cursor-help"
                                            title={t('databaseStudio.structure.tooltips.unique') || undefined}
                                          >
                                            <Shield className="w-2.5 h-2.5" /> UQ
                                          </span>
                                        )}
                                        {isIDX && !isPK && !isUQ && (
                                          <span 
                                            className="px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-600 border border-emerald-500/20 text-[10px] font-extrabold uppercase tracking-wide flex items-center gap-1 cursor-help"
                                            title={t('databaseStudio.structure.tooltips.index') || undefined}
                                          >
                                            <Search className="w-2.5 h-2.5" /> IDX
                                          </span>
                                        )}
                                        {!isPK && !fk && !isUQ && !isIDX && (
                                          <span className="text-[10px] text-muted-foreground/40 italic">—</span>
                                        )}
                                      </div>
                                    </td>
                                    <td className="py-3 px-4 font-mono text-muted-foreground">{col.default === null ? <span className="text-muted-foreground/30 italic">NULL</span> : String(col.default)}</td>
                                    <td className="py-3 px-4 text-right">
                                      <DropdownMenu>
                                        <DropdownMenuTrigger>
                                          <Button variant="ghost" size="icon" className="h-8 w-8 hover:bg-muted/50 cursor-pointer" style={{ cursor: 'pointer' }}>
                                            <MoreHorizontal className="w-4 h-4" />
                                          </Button>
                                        </DropdownMenuTrigger>
                                        <DropdownMenuContent align="end" className="w-36 bg-card border border-border/80 rounded-xl shadow-xl">
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
                                )
                              })}
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
          {designerAction === 'create_table' && (
            <Dialog open={designerAction === 'create_table'} onOpenChange={(open: boolean) => !open && setDesignerAction(null)}>
            <DialogContent className="sm:max-w-md bg-card border border-border/80 rounded-xl shadow-2xl">
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
          )}

          {/* Add Column Dialog Modal */}
          {designerAction === 'add_column' && (
            <Dialog open={designerAction === 'add_column'} onOpenChange={(open: boolean) => !open && resetAddColumnForm()}>
            <DialogContent className="sm:max-w-lg max-h-[85vh] overflow-y-auto bg-card border border-border/80 rounded-xl shadow-2xl scrollbar-thin">
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
                    <Select
                      value={newColType}
                      onValueChange={(val) => {
                        setNewColType(val || '');
                        setNewColDefault('');
                      }}
                    >
                      <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold text-left justify-between">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent className="max-h-[300px]">
                        <SelectGroup>
                          <SelectLabel className="px-3 py-1 text-[9px] font-bold uppercase tracking-wider text-muted-foreground/60 bg-muted/5">Text</SelectLabel>
                          <SelectItem value="varchar" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">VARCHAR</SelectItem>
                          <SelectItem value="text" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">TEXT</SelectItem>
                          <SelectItem value="char" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">CHAR</SelectItem>
                        </SelectGroup>
                        <SelectSeparator className="bg-border/40" />
                        <SelectGroup>
                          <SelectLabel className="px-3 py-1 text-[9px] font-bold uppercase tracking-wider text-muted-foreground/60 bg-muted/5">Numeric</SelectLabel>
                          <SelectItem value="integer" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">INTEGER</SelectItem>
                          <SelectItem value="bigint" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">BIGINT</SelectItem>
                          <SelectItem value="decimal" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">DECIMAL</SelectItem>
                          <SelectItem value="float" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">FLOAT</SelectItem>
                          <SelectItem value="double" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">DOUBLE</SelectItem>
                        </SelectGroup>
                        <SelectSeparator className="bg-border/40" />
                        <SelectGroup>
                          <SelectLabel className="px-3 py-1 text-[9px] font-bold uppercase tracking-wider text-muted-foreground/60 bg-muted/5">Temporal</SelectLabel>
                          <SelectItem value="timestamp" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">TIMESTAMP</SelectItem>
                          <SelectItem value="datetime" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">DATETIME</SelectItem>
                          <SelectItem value="date" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">DATE</SelectItem>
                          <SelectItem value="time" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">TIME</SelectItem>
                        </SelectGroup>
                        <SelectSeparator className="bg-border/40" />
                        <SelectGroup>
                          <SelectLabel className="px-3 py-1 text-[9px] font-bold uppercase tracking-wider text-muted-foreground/60 bg-muted/5">Logical</SelectLabel>
                          <SelectItem value="boolean" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">BOOLEAN</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
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

                {/* Default Value Input & Helper Presets */}
                <div className="space-y-2">
                  <div className="space-y-1.5">
                    <Label htmlFor="new_col_default" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                      {t('databaseStudio.structure.createTableDialog.defaultValue')}
                    </Label>
                    <Input
                      id="new_col_default"
                      value={newColDefault}
                      onChange={(e) => setNewColDefault(e.target.value)}
                      placeholder="e.g. NULL, 'active', 0"
                      className="h-10 rounded-xl bg-background/50 text-xs font-mono"
                    />
                  </div>
                  {/* Preset Chips */}
                  <div className="flex flex-wrap gap-1.5 items-center">
                    <span className="text-[9px] font-bold text-muted-foreground uppercase tracking-wider mr-1">Presets:</span>
                    {(() => {
                      let chips = ['NULL'];
                      const normType = newColType.toLowerCase();
                      if (normType === 'varchar' || normType === 'text' || normType === 'char') {
                        chips = ["''", 'NULL'];
                      } else if (['integer', 'bigint', 'decimal', 'float', 'double'].includes(normType)) {
                        chips = ['0', '1', 'NULL'];
                      } else if (['timestamp', 'datetime'].includes(normType)) {
                        chips = ['CURRENT_TIMESTAMP', 'NULL'];
                      } else if (normType === 'boolean') {
                        chips = ['TRUE', 'FALSE', 'NULL'];
                      }
                      return chips.map(chip => (
                        <button
                          key={chip}
                          type="button"
                          onClick={() => setNewColDefault(chip)}
                          className="px-2 py-0.5 rounded text-[10px] font-mono font-bold bg-muted/60 border border-border/40 hover:bg-muted/100 hover:border-border transition-colors cursor-pointer text-muted-foreground hover:text-foreground"
                          style={{ cursor: 'pointer' }}
                        >
                          {chip}
                        </button>
                      ));
                    })()}
                  </div>
                </div>

                {/* Constraint Cards (Nullable, PK, Unique) */}
                <div className="space-y-1.5 border-t border-border/40 pt-3">
                  <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.designer.configHeader')}</Label>
                  <div className="grid grid-cols-3 gap-2">
                    {/* Nullable Card */}
                    <button
                      type="button"
                      onClick={() => setNewColNullable(!newColNullable)}
                      className={cn(
                        "flex flex-col items-center justify-center p-3 rounded-xl border transition-all duration-200 cursor-pointer text-center gap-1.5",
                        newColNullable 
                          ? "bg-emerald-500/10 border-emerald-500/30 text-emerald-500 shadow-sm shadow-emerald-500/5" 
                          : "bg-muted/30 border-border/40 text-muted-foreground hover:bg-muted/50"
                      )}
                      style={{ cursor: 'pointer' }}
                    >
                      <span className="text-[10px] font-extrabold uppercase tracking-wide">Nullable</span>
                      <span className="text-[9px] opacity-75 font-medium leading-tight">{t('databaseStudio.structure.designer.nullableDesc')}</span>
                    </button>
                    
                    {/* Primary Key Card */}
                    <button
                      type="button"
                      onClick={() => setNewColPk(!newColPk)}
                      className={cn(
                        "flex flex-col items-center justify-center p-3 rounded-xl border transition-all duration-200 cursor-pointer text-center gap-1.5",
                        newColPk 
                          ? "bg-purple-500/10 border-purple-500/30 text-purple-500 shadow-sm shadow-purple-500/5" 
                          : "bg-muted/30 border-border/40 text-muted-foreground hover:bg-muted/50"
                      )}
                      style={{ cursor: 'pointer' }}
                    >
                      <span className="text-[10px] font-extrabold uppercase tracking-wide">Primary Key</span>
                      <span className="text-[9px] opacity-75 font-medium leading-tight">{t('databaseStudio.structure.designer.pkDesc')}</span>
                    </button>

                    {/* Unique Card */}
                    <button
                      type="button"
                      onClick={() => setNewColUnique(!newColUnique)}
                      className={cn(
                        "flex flex-col items-center justify-center p-3 rounded-xl border transition-all duration-200 cursor-pointer text-center gap-1.5",
                        newColUnique 
                          ? "bg-orange-500/10 border-orange-500/30 text-orange-500 shadow-sm shadow-orange-500/5" 
                          : "bg-muted/30 border-border/40 text-muted-foreground hover:bg-muted/50"
                      )}
                      style={{ cursor: 'pointer' }}
                    >
                      <span className="text-[10px] font-extrabold uppercase tracking-wide">Unique</span>
                      <span className="text-[9px] opacity-75 font-medium leading-tight">{t('databaseStudio.structure.designer.uniqueDesc')}</span>
                    </button>
                  </div>
                </div>

                {/* Collapsible Foreign Key Relation Panel */}
                <div className="rounded-xl border border-border/60 bg-muted/5 overflow-hidden">
                  <button
                    type="button"
                    onClick={() => {
                      const active = !newColFk;
                      setNewColFk(active);
                      if (active && !newColFkTargetTable && schemaData.length > 0) {
                        const firstTable = schemaData[0].name;
                        setNewColFkTargetTable(firstTable);
                        const firstCol = schemaData[0].columns?.[0]?.name || '';
                        setNewColFkTargetColumn(firstCol);
                      }
                    }}
                    className="w-full flex items-center justify-between px-4 py-3 bg-muted/20 border-b border-border/45 hover:bg-muted/40 transition-colors cursor-pointer text-left"
                    style={{ cursor: 'pointer' }}
                  >
                    <div className="flex items-center gap-2">
                      <Link className={cn("w-4 h-4 transition-colors", newColFk ? "text-blue-500" : "text-muted-foreground")} />
                      <span className="text-xs font-bold uppercase tracking-wider text-foreground/80">{t('databaseStudio.structure.designer.fkRelation')}</span>
                    </div>
                    <span className={cn(
                      "text-[9px] font-black px-2 py-0.5 rounded border transition-all",
                      newColFk ? "bg-blue-500/10 text-blue-500 border-blue-500/25" : "bg-muted text-muted-foreground border-border/40"
                    )}>
                      {newColFk ? t('databaseStudio.structure.designer.active') : t('databaseStudio.structure.designer.inactive')}
                    </span>
                  </button>
                  
                  {newColFk && (
                    <div className="p-4 space-y-3 bg-background/20 animate-in fade-in duration-200">
                      <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                          <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.designer.targetTable')}</Label>
                          <Select
                            value={newColFkTargetTable}
                            onValueChange={(val) => {
                              setNewColFkTargetTable(val || '');
                              const firstCol = schemaData.find(t => t.name === (val || ''))?.columns?.[0]?.name || '';
                              setNewColFkTargetColumn(firstCol);
                            }}
                          >
                            <SelectTrigger className="w-full h-9 px-2.5 rounded-lg border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold text-left justify-between">
                              <SelectValue placeholder={t('databaseStudio.structure.designer.selectTable') || undefined} />
                            </SelectTrigger>
                            <SelectContent className="max-h-[250px]">
                              <SelectItem value="" className="py-2 px-3 pl-8 text-xs font-medium text-muted-foreground cursor-pointer">
                                {t('databaseStudio.structure.designer.selectTable')}
                              </SelectItem>
                              {schemaData.map((t: any) => (
                                <SelectItem key={t.name} value={t.name} className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">
                                  {t.name}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                        
                        <div className="space-y-1.5">
                          <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.designer.targetColumn')}</Label>
                          <Select
                            value={newColFkTargetColumn}
                            onValueChange={(val) => setNewColFkTargetColumn(val || '')}
                            disabled={!newColFkTargetTable}
                          >
                            <SelectTrigger className="w-full h-9 px-2.5 rounded-lg border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold text-left justify-between">
                              <SelectValue placeholder={t('databaseStudio.structure.designer.selectColumn') || undefined} />
                            </SelectTrigger>
                            <SelectContent className="max-h-[250px]">
                              <SelectItem value="" className="py-2 px-3 pl-8 text-xs font-medium text-muted-foreground cursor-pointer">
                                {t('databaseStudio.structure.designer.selectColumn')}
                              </SelectItem>
                              {(schemaData.find(t => t.name === newColFkTargetTable)?.columns || []).map((c: any) => (
                                <SelectItem key={c.name} value={c.name} className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">
                                  {c.name}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                      </div>
                      
                      <div className="space-y-1.5">
                        <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.designer.onDeleteAction')}</Label>
                        <Select
                          value={newColFkOnDelete}
                          onValueChange={(val) => setNewColFkOnDelete(val || '')}
                        >
                          <SelectTrigger className="w-full h-9 px-2.5 rounded-lg border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold text-left justify-between">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="CASCADE" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">{t('databaseStudio.structure.designer.cascadeDesc')}</SelectItem>
                            <SelectItem value="SET NULL" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">{t('databaseStudio.structure.designer.setNullDesc')}</SelectItem>
                            <SelectItem value="RESTRICT" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">{t('databaseStudio.structure.designer.restrictDesc')}</SelectItem>
                            <SelectItem value="NO ACTION" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">{t('databaseStudio.structure.designer.noActionDesc')}</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                    </div>
                  )}
                </div>

                {/* Column Description Textarea */}
                <div className="space-y-1.5">
                  <Label htmlFor="new_col_comment" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.designer.commentLabel')}</Label>
                  <textarea
                    id="new_col_comment"
                    value={newColComment}
                    onChange={(e) => setNewColComment(e.target.value)}
                    placeholder={t('databaseStudio.structure.designer.commentPlaceholder') || undefined}
                    className="w-full min-h-[60px] max-h-[120px] p-2.5 rounded-xl border border-border bg-background/50 hover:bg-background/80 text-xs transition-colors outline-none focus:border-primary/50 resize-y"
                  />
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
          )}

          {/* Modify Column Dialog Modal */}
          {designerAction === 'modify_column' && (
            <Dialog open={designerAction === 'modify_column'} onOpenChange={(open: boolean) => !open && resetModifyColumnForm()}>
            <DialogContent className="sm:max-w-lg max-h-[85vh] overflow-y-auto bg-card border border-border/80 rounded-xl shadow-2xl scrollbar-thin">
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
                      <Select
                        value={editColType}
                        onValueChange={(val) => {
                          setEditColType(val || '');
                          setEditColDefault('');
                        }}
                      >
                        <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold text-left justify-between">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent className="max-h-[300px]">
                          <SelectGroup>
                            <SelectLabel className="px-3 py-1 text-[9px] font-bold uppercase tracking-wider text-muted-foreground/60 bg-muted/5">Text</SelectLabel>
                            <SelectItem value="varchar" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">VARCHAR</SelectItem>
                            <SelectItem value="text" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">TEXT</SelectItem>
                            <SelectItem value="char" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">CHAR</SelectItem>
                          </SelectGroup>
                          <SelectSeparator className="bg-border/40" />
                          <SelectGroup>
                            <SelectLabel className="px-3 py-1 text-[9px] font-bold uppercase tracking-wider text-muted-foreground/60 bg-muted/5">Numeric</SelectLabel>
                            <SelectItem value="integer" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">INTEGER</SelectItem>
                            <SelectItem value="bigint" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">BIGINT</SelectItem>
                            <SelectItem value="decimal" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">DECIMAL</SelectItem>
                            <SelectItem value="float" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">FLOAT</SelectItem>
                            <SelectItem value="double" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">DOUBLE</SelectItem>
                          </SelectGroup>
                          <SelectSeparator className="bg-border/40" />
                          <SelectGroup>
                            <SelectLabel className="px-3 py-1 text-[9px] font-bold uppercase tracking-wider text-muted-foreground/60 bg-muted/5">Temporal</SelectLabel>
                            <SelectItem value="timestamp" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">TIMESTAMP</SelectItem>
                            <SelectItem value="datetime" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">DATETIME</SelectItem>
                            <SelectItem value="date" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">DATE</SelectItem>
                            <SelectItem value="time" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">TIME</SelectItem>
                          </SelectGroup>
                          <SelectSeparator className="bg-border/40" />
                          <SelectGroup>
                            <SelectLabel className="px-3 py-1 text-[9px] font-bold uppercase tracking-wider text-muted-foreground/60 bg-muted/5">Logical</SelectLabel>
                            <SelectItem value="boolean" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">BOOLEAN</SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
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

                  {/* Default Value Input & Helper Presets */}
                  <div className="space-y-2">
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
                    {/* Preset Chips */}
                    <div className="flex flex-wrap gap-1.5 items-center">
                      <span className="text-[9px] font-bold text-muted-foreground uppercase tracking-wider mr-1">Presets:</span>
                      {(() => {
                        let chips = ['NULL'];
                        const normType = editColType.toLowerCase();
                        if (normType === 'varchar' || normType === 'text' || normType === 'char') {
                          chips = ["''", 'NULL'];
                        } else if (['integer', 'bigint', 'decimal', 'float', 'double'].includes(normType)) {
                          chips = ['0', '1', 'NULL'];
                        } else if (['timestamp', 'datetime'].includes(normType)) {
                          chips = ['CURRENT_TIMESTAMP', 'NULL'];
                        } else if (normType === 'boolean') {
                          chips = ['TRUE', 'FALSE', 'NULL'];
                        }
                        return chips.map(chip => (
                          <button
                            key={chip}
                            type="button"
                            onClick={() => setEditColDefault(chip)}
                            className="px-2 py-0.5 rounded text-[10px] font-mono font-bold bg-muted/60 border border-border/40 hover:bg-muted/100 hover:border-border transition-colors cursor-pointer text-muted-foreground hover:text-foreground"
                            style={{ cursor: 'pointer' }}
                          >
                            {chip}
                          </button>
                        ));
                      })()}
                    </div>
                  </div>

                  {/* Constraint Cards (Nullable, PK, Unique) */}
                  <div className="space-y-1.5 border-t border-border/40 pt-3">
                    <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.designer.configHeader')}</Label>
                    <div className="grid grid-cols-3 gap-2">
                      {/* Nullable Card */}
                      <button
                        type="button"
                        onClick={() => setEditColNullable(!editColNullable)}
                        className={cn(
                          "flex flex-col items-center justify-center p-3 rounded-xl border transition-all duration-200 cursor-pointer text-center gap-1.5",
                          editColNullable 
                            ? "bg-emerald-500/10 border-emerald-500/30 text-emerald-500 shadow-sm shadow-emerald-500/5" 
                            : "bg-muted/30 border-border/40 text-muted-foreground hover:bg-muted/50"
                        )}
                        style={{ cursor: 'pointer' }}
                      >
                        <span className="text-[10px] font-extrabold uppercase tracking-wide">Nullable</span>
                        <span className="text-[9px] opacity-75 font-medium leading-tight">{t('databaseStudio.structure.designer.nullableDesc')}</span>
                      </button>
                      
                      {/* Primary Key Card */}
                      <button
                        type="button"
                        onClick={() => setEditColPk(!editColPk)}
                        className={cn(
                          "flex flex-col items-center justify-center p-3 rounded-xl border transition-all duration-200 cursor-pointer text-center gap-1.5",
                          editColPk 
                            ? "bg-purple-500/10 border-purple-500/30 text-purple-500 shadow-sm shadow-purple-500/5" 
                            : "bg-muted/30 border-border/40 text-muted-foreground hover:bg-muted/50"
                        )}
                        style={{ cursor: 'pointer' }}
                      >
                        <span className="text-[10px] font-extrabold uppercase tracking-wide">Primary Key</span>
                        <span className="text-[9px] opacity-75 font-medium leading-tight">{t('databaseStudio.structure.designer.pkDesc')}</span>
                      </button>

                      {/* Unique Card */}
                      <button
                        type="button"
                        onClick={() => setEditColUnique(!editColUnique)}
                        className={cn(
                          "flex flex-col items-center justify-center p-3 rounded-xl border transition-all duration-200 cursor-pointer text-center gap-1.5",
                          editColUnique 
                            ? "bg-orange-500/10 border-orange-500/30 text-orange-500 shadow-sm shadow-orange-500/5" 
                            : "bg-muted/30 border-border/40 text-muted-foreground hover:bg-muted/50"
                        )}
                        style={{ cursor: 'pointer' }}
                      >
                        <span className="text-[10px] font-extrabold uppercase tracking-wide">Unique</span>
                        <span className="text-[9px] opacity-75 font-medium leading-tight">{t('databaseStudio.structure.designer.uniqueDesc')}</span>
                      </button>
                    </div>
                  </div>

                  {/* Collapsible Foreign Key Relation Panel */}
                  <div className="rounded-xl border border-border/60 bg-muted/5 overflow-hidden">
                    <button
                      type="button"
                      onClick={() => {
                        const active = !editColFk;
                        setEditColFk(active);
                        if (active && !editColFkTargetTable && schemaData.length > 0) {
                          const firstTable = schemaData[0].name;
                          setEditColFkTargetTable(firstTable);
                          const firstCol = schemaData[0].columns?.[0]?.name || '';
                          setEditColFkTargetColumn(firstCol);
                        }
                      }}
                      className="w-full flex items-center justify-between px-4 py-3 bg-muted/20 border-b border-border/45 hover:bg-muted/40 transition-colors cursor-pointer text-left"
                      style={{ cursor: 'pointer' }}
                    >
                      <div className="flex items-center gap-2">
                        <Link className={cn("w-4 h-4 transition-colors", editColFk ? "text-blue-500" : "text-muted-foreground")} />
                        <span className="text-xs font-bold uppercase tracking-wider text-foreground/80">{t('databaseStudio.structure.designer.fkRelation')}</span>
                      </div>
                      <span className={cn(
                        "text-[9px] font-black px-2 py-0.5 rounded border transition-all",
                        editColFk ? "bg-blue-500/10 text-blue-500 border-blue-500/25" : "bg-muted text-muted-foreground border-border/40"
                      )}>
                        {editColFk ? t('databaseStudio.structure.designer.active') : t('databaseStudio.structure.designer.inactive')}
                      </span>
                    </button>
                    
                    {editColFk && (
                      <div className="p-4 space-y-3 bg-background/20 animate-in fade-in duration-200">
                        <div className="grid grid-cols-2 gap-3">
                          <div className="space-y-1.5">
                            <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.designer.targetTable')}</Label>
                            <Select
                              value={editColFkTargetTable}
                              onValueChange={(val) => {
                                setEditColFkTargetTable(val || '');
                                const firstCol = schemaData.find(t => t.name === (val || ''))?.columns?.[0]?.name || '';
                                setEditColFkTargetColumn(firstCol);
                              }}
                            >
                              <SelectTrigger className="w-full h-9 px-2.5 rounded-lg border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold text-left justify-between">
                                <SelectValue placeholder={t('databaseStudio.structure.designer.selectTable') || undefined} />
                              </SelectTrigger>
                              <SelectContent className="max-h-[250px]">
                                <SelectItem value="" className="py-2 px-3 pl-8 text-xs font-medium text-muted-foreground cursor-pointer">
                                  {t('databaseStudio.structure.designer.selectTable')}
                                </SelectItem>
                                {schemaData.map((t: any) => (
                                  <SelectItem key={t.name} value={t.name} className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">
                                    {t.name}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </div>
                          
                          <div className="space-y-1.5">
                            <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.designer.targetColumn')}</Label>
                            <Select
                              value={editColFkTargetColumn}
                              onValueChange={(val) => setEditColFkTargetColumn(val || '')}
                              disabled={!editColFkTargetTable}
                            >
                              <SelectTrigger className="w-full h-9 px-2.5 rounded-lg border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold text-left justify-between">
                                <SelectValue placeholder={t('databaseStudio.structure.designer.selectColumn') || undefined} />
                              </SelectTrigger>
                              <SelectContent className="max-h-[250px]">
                                <SelectItem value="" className="py-2 px-3 pl-8 text-xs font-medium text-muted-foreground cursor-pointer">
                                  {t('databaseStudio.structure.designer.selectColumn')}
                                </SelectItem>
                                {(schemaData.find(t => t.name === editColFkTargetTable)?.columns || []).map((c: any) => (
                                  <SelectItem key={c.name} value={c.name} className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">
                                    {c.name}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </div>
                        </div>
                        
                        <div className="space-y-1.5">
                          <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.designer.onDeleteAction')}</Label>
                          <Select
                            value={editColFkOnDelete}
                            onValueChange={(val) => setEditColFkOnDelete(val || '')}
                          >
                            <SelectTrigger className="w-full h-9 px-2.5 rounded-lg border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold text-left justify-between">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="CASCADE" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">{t('databaseStudio.structure.designer.cascadeDesc')}</SelectItem>
                              <SelectItem value="SET NULL" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">{t('databaseStudio.structure.designer.setNullDesc')}</SelectItem>
                              <SelectItem value="RESTRICT" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">{t('databaseStudio.structure.designer.restrictDesc')}</SelectItem>
                              <SelectItem value="NO ACTION" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">{t('databaseStudio.structure.designer.noActionDesc')}</SelectItem>
                            </SelectContent>
                          </Select>
                        </div>
                      </div>
                    )}
                  </div>

                  {/* Column Description Textarea */}
                  <div className="space-y-1.5">
                    <Label htmlFor="edit_col_comment" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.structure.designer.commentLabel')}</Label>
                    <textarea
                      id="edit_col_comment"
                      value={editColComment}
                      onChange={(e) => setEditColComment(e.target.value)}
                      placeholder={t('databaseStudio.structure.designer.commentPlaceholder') || undefined}
                      className="w-full min-h-[60px] max-h-[120px] p-2.5 rounded-xl border border-border bg-background/50 hover:bg-background/80 text-xs transition-colors outline-none focus:border-primary/50 resize-y"
                    />
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
          )}
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
                    <Select
                      value=""
                      onValueChange={(val) => {
                        if (!val) return
                        
                        const tableName = selectedTable || (schemaData.length > 0 ? schemaData[0].name : 'users')
                        const columns = schemaData.find(t => t.name === tableName)?.columns || []
                        const firstColName = columns.length > 0 ? columns[0].name : 'id'

                        const isPostgres = dbOverview?.engine?.toLowerCase() === 'postgres'
                        const q = isPostgres ? '"' : '`'

                        let sql = ''
                        if (val === 'select') {
                          sql = `SELECT * FROM ${q}${tableName}${q} LIMIT 10;`
                        } else if (val === 'count') {
                          sql = `SELECT COUNT(*) AS total_rows FROM ${q}${tableName}${q};`
                        } else if (val === 'filter') {
                          sql = `SELECT * FROM ${q}${tableName}${q} WHERE ${q}${firstColName}${q} = 1;`
                        } else if (val === 'group') {
                          sql = `SELECT ${q}${firstColName}${q}, COUNT(*) AS count FROM ${q}${tableName}${q} GROUP BY ${q}${firstColName}${q} ORDER BY count DESC;`
                        }
                        
                        if (sql) {
                          setSqlQuery(sql)
                        }
                      }}
                    >
                      <SelectTrigger className="h-10 px-3.5 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold text-left justify-between gap-2 cursor-pointer">
                        <SelectValue placeholder={t('databaseStudio.query.templates.label') || undefined} />
                      </SelectTrigger>
                      <SelectContent alignItemWithTrigger={false} className="w-auto min-w-[220px]">
                        <SelectItem value="select" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">{t('databaseStudio.query.templates.select')}</SelectItem>
                        <SelectItem value="count" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">{t('databaseStudio.query.templates.count')}</SelectItem>
                        <SelectItem value="filter" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">{t('databaseStudio.query.templates.filter')}</SelectItem>
                        <SelectItem value="group" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">{t('databaseStudio.query.templates.group')}</SelectItem>
                      </SelectContent>
                    </Select>
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
                                        {formatCellValue(row[col])}
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
