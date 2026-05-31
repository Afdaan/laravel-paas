import React, { useState, useEffect, useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import {
  Search,
  Table,
  Plus,
  MoreHorizontal,
  Pencil,
  Trash2,
  PlusCircle,
  Clock,
  Copy,
  Check
} from 'lucide-react'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { databaseAPI } from '../../services/api'
import { useStudio, SchemaColumn } from './StudioContext'
import {
  formatCellValue,
  adjustDatetimeForDatabase,
  formatDatetimeLocal,
  formatDate,
  toLocalDateString,
  toLocalISOString
} from './utils'

interface TableDataGrid {
  columns: string[];
  rows: Record<string, unknown>[];
}

const tryFormatJson = (val: string): string => {
  try {
    JSON.parse(val)
    return val
  } catch (e) {
    return JSON.stringify(val)
  }
}

export function StudioTablesTab() {
  const {
    id,
    dbOverview,
    schemaData,
    loadStudioData,
    isActionLoading,
    setIsActionLoading,
    triggerConfirmation,
    t
  } = useStudio()

  const [searchParams, setSearchParams] = useSearchParams()
  const selectedTable = searchParams.get('table') || ''
  const setSelectedTable = (table: string) => {
    setSearchParams(prev => {
      if (table) {
        prev.set('table', table)
      } else {
        prev.delete('table')
      }
      return prev
    }, { replace: true })
  }
  const [tableData, setTableData] = useState<TableDataGrid | null>(null)
  const [tablePage, setTablePage] = useState(1)
  const [tableLimit] = useState(25)
  const [tableTotal, setTableTotal] = useState(0)
  const [tableSearch, setTableSearch] = useState('')

  // Visual Dynamic Insert Row states
  const [showInsertModal, setShowInsertModal] = useState(false)
  const [insertFormData, setInsertFormData] = useState<Record<string, string | number | boolean | null>>({})
  const [showRowInsertPreview, setShowRowInsertPreview] = useState(false)
  const [rowInsertPreviewSql, setRowInsertPreviewSql] = useState('')
  const [copiedSql, setCopiedSql] = useState(false)

  // Visual Edit Row states
  const [showEditModal, setShowEditModal] = useState(false)
  const [editingRow, setEditingRow] = useState<Record<string, unknown> | null>(null)
  const [editFormData, setEditFormData] = useState<Record<string, string | number | boolean | null>>({})
  const [showRowEditPreview, setShowRowEditPreview] = useState(false)
  const [rowEditPreviewSql, setRowEditPreviewSql] = useState('')

  // Set default selected table on schemaData load
  useEffect(() => {
    if (schemaData.length > 0 && !selectedTable) {
      setSelectedTable(schemaData[0].name)
    }
  }, [schemaData, selectedTable])

  const filteredTables = schemaData.filter(tb => 
    tb.name.toLowerCase().includes(tableSearch.toLowerCase())
  )

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
    } catch (error) {
      toast.error(t('databaseStudio.errors.readRowsFailed'))
    }
  }, [id, selectedTable, tablePage, tableLimit, t])

  useEffect(() => {
    if (selectedTable) {
      loadTableDataGrid()
    }
  }, [selectedTable, tablePage, loadTableDataGrid])

  const handleDeleteRow = (row: Record<string, unknown>, pkCol: string) => {
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
        } catch (error) {
          const err = error as { response?: { data?: { error?: string } } }
          toast.error(err.response?.data?.error || t('databaseStudio.errors.deleteRowFailed'))
        } finally {
          setIsActionLoading(false)
        }
      }
    })
  }

  const handleCopySql = (sql: string) => {
    navigator.clipboard.writeText(sql)
    setCopiedSql(true)
    toast.success(t('common.copied') || 'Copied to clipboard!')
    setTimeout(() => setCopiedSql(false), 2000)
  }

  const handleInsertRowSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!id || !selectedTable) return

    try {
      const cols = schemaData.find(tb => tb.name === selectedTable)?.columns || []
      const fields: string[] = []
      const values: string[] = []

      const isPostgres = dbOverview?.engine?.toLowerCase() === 'postgres'
      const q = isPostgres ? '"' : '`'

      cols.forEach((col: SchemaColumn) => {
        if (col.key === 'PRI' && !insertFormData[col.name]) return

        let val = insertFormData[col.name]
        if (val === undefined || val === '') return

        fields.push(`${q}${col.name}${q}`)

        const typeLower = col.type.toLowerCase()
        if (typeLower.includes('timestamp') || typeLower.includes('datetime')) {
          val = adjustDatetimeForDatabase(String(val))
        }

        if (typeLower.includes('bool') || typeLower.includes('tinyint(1)')) {
          const boolVal = val === 'true' || val === true || val === '1' || val === 1
          values.push(isPostgres ? (boolVal ? 'TRUE' : 'FALSE') : (boolVal ? '1' : '0'))
        } else if (typeLower.includes('int') || typeLower.includes('decimal') || typeLower.includes('float') || typeLower.includes('double')) {
          const num = Number(val)
          values.push(isNaN(num) ? '0' : String(num))
        } else {
          let finalVal = String(val)
          if (typeLower.includes('json') || col.name.toLowerCase().includes('json')) {
            finalVal = tryFormatJson(finalVal)
          }
          const escapedVal = finalVal.replace(/'/g, "''")
          values.push(`'${escapedVal}'`)
        }
      })

      if (fields.length === 0) {
        toast.error(t('databaseStudio.errors.designerActionFailed'))
        return
      }

      const sql = `INSERT INTO ${q}${selectedTable}${q} (${fields.join(', ')}) VALUES (${values.join(', ')});`
      setRowInsertPreviewSql(sql)
      setShowRowInsertPreview(true)
    } catch (error) {
      const err = error as { message?: string }
      toast.error(t('databaseStudio.tables.insertModal.failed') + ': ' + err.message)
    }
  }

  const handleInsertRowCommit = async () => {
    if (!id || !selectedTable || !rowInsertPreviewSql) return

    setIsActionLoading(true)
    try {
      const res = await databaseAPI.query(id, rowInsertPreviewSql)
      if (res.data && res.data.error) {
        throw new Error(res.data.error)
      }

      toast.success(t('databaseStudio.tables.insertModal.success'))
      setShowInsertModal(false)
      loadTableDataGrid()
      loadStudioData()
    } catch (error) {
      const err = error as { response?: { data?: { error?: string } }; message?: string }
      toast.error(t('databaseStudio.tables.insertModal.failed') + ': ' + (err.response?.data?.error || err.message))
    } finally {
      setIsActionLoading(false)
    }
  }

  const generateRowEditSql = () => {
    if (!selectedTable || !editingRow || !tableData) return ''
    const pkColumn = tableData.columns.find((c: string) => 
      c.toLowerCase() === 'id' || c.toLowerCase() === 'uid' || c.toLowerCase() === 'uuid'
    ) || tableData.columns[0]
    const pkValue = editingRow[pkColumn]

    const isPostgres = dbOverview?.engine?.toLowerCase() === 'postgres'
    const q = isPostgres ? '"' : '`'

    const setClauses: string[] = []
    const cols = schemaData.find(tb => tb.name === selectedTable)?.columns || []

    cols.forEach((col: SchemaColumn) => {
      if (col.name.toLowerCase() === pkColumn.toLowerCase()) return

      const typeLower = col.type.toLowerCase()
      let val = editFormData[col.name]
      const escapedCol = `${q}${col.name}${q}`

      if (val === undefined || val === '') {
        if (col.nullable === 'YES' || col.nullable === true) {
          setClauses.push(`${escapedCol} = NULL`)
        } else {
          if (typeLower.includes('bool') || typeLower.includes('tinyint(1)')) {
            setClauses.push(`${escapedCol} = false`)
          } else if (typeLower.includes('int') || typeLower.includes('decimal') || typeLower.includes('float') || typeLower.includes('double')) {
            setClauses.push(`${escapedCol} = 0`)
          } else {
            setClauses.push(`${escapedCol} = ''`)
          }
        }
        return
      }

      if (typeLower.includes('timestamp') || typeLower.includes('datetime')) {
        val = adjustDatetimeForDatabase(String(val))
      }

      if (typeLower.includes('bool') || typeLower.includes('tinyint(1)')) {
        const boolVal = val === 'true' || val === true || val === '1' || val === 1
        setClauses.push(`${escapedCol} = ${isPostgres ? (boolVal ? 'TRUE' : 'FALSE') : (boolVal ? '1' : '0')}`)
      } else if (typeLower.includes('int') || typeLower.includes('decimal') || typeLower.includes('float') || typeLower.includes('double')) {
        const num = Number(val)
        setClauses.push(`${escapedCol} = ${isNaN(num) ? 0 : num}`)
      } else {
        let finalVal = String(val)
        if (typeLower.includes('json') || col.name.toLowerCase().includes('json')) {
          finalVal = tryFormatJson(finalVal)
        }
        const escapedVal = finalVal.replace(/'/g, "''")
        setClauses.push(`${escapedCol} = '${escapedVal}'`)
      }
    })

    const escapedPkValue = typeof pkValue === 'number' ? pkValue : `'${String(pkValue).replace(/'/g, "''")}'`
    return `UPDATE ${q}${selectedTable}${q} SET ${setClauses.join(', ')} WHERE ${q}${pkColumn}${q} = ${escapedPkValue};`
  }

  const openEditRowModal = (row: Record<string, unknown>) => {
    setEditingRow(row)
    
    const cols = schemaData.find(tb => tb.name === selectedTable)?.columns || []
    const initialData: Record<string, string | number | boolean | null> = {}
    cols.forEach((c: SchemaColumn) => {
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
        initialData[c.name] = val as string | number | boolean | null
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
    if (!id || !selectedTable || !editingRow || !tableData) return

    const pkColumn = tableData.columns.find((c: string) => 
      c.toLowerCase() === 'id' || c.toLowerCase() === 'uid' || c.toLowerCase() === 'uuid'
    ) || tableData.columns[0]

    const pkValue = editingRow[pkColumn]
    if (pkValue == null) {
      toast.error(t('databaseStudio.errors.missingPrimaryKey'))
      return
    }

    setIsActionLoading(true)
    try {
      const cols = schemaData.find(tb => tb.name === selectedTable)?.columns || []
      const updates: Record<string, string | number | boolean | null> = {}

      cols.forEach((col: SchemaColumn) => {
        if (col.name.toLowerCase() === pkColumn.toLowerCase()) return

        const typeLower = col.type.toLowerCase()
        let val = editFormData[col.name]
        if (val === undefined || val === '') {
          if (col.nullable === 'YES' || col.nullable === true) {
            updates[col.name] = null
          } else {
            if (typeLower.includes('bool') || typeLower.includes('tinyint(1)')) {
              updates[col.name] = false
            } else if (typeLower.includes('int') || typeLower.includes('decimal') || typeLower.includes('float') || typeLower.includes('double')) {
              updates[col.name] = 0
            } else {
              updates[col.name] = ""
            }
          }
          return
        }

        if (typeLower.includes('timestamp') || typeLower.includes('datetime')) {
          val = adjustDatetimeForDatabase(String(val))
        }

        if (typeLower.includes('bool') || typeLower.includes('tinyint(1)')) {
          updates[col.name] = val === 'true' || val === true || val === '1' || val === 1
        } else if (typeLower.includes('int') || typeLower.includes('decimal') || typeLower.includes('float') || typeLower.includes('double')) {
          const num = Number(val)
          updates[col.name] = isNaN(num) ? 0 : num
        } else {
          let finalVal = val
          if (typeLower.includes('json') || col.name.toLowerCase().includes('json')) {
            finalVal = tryFormatJson(String(val))
          }
          updates[col.name] = finalVal
        }
      })

      await databaseAPI.updateRow(id, selectedTable, pkColumn, pkValue, updates)
      toast.success(t('databaseStudio.tables.editModal.success'))
      setShowEditModal(false)
      loadTableDataGrid()
      loadStudioData()
    } catch (error) {
      const err = error as { response?: { data?: { error?: string } }; message?: string }
      toast.error(t('databaseStudio.tables.editModal.failed') + ': ' + (err.response?.data?.error || err.message))
    } finally {
      setIsActionLoading(false)
    }
  }

  const instanceStatus = dbOverview?.status || 'active'
  const isSuspended = instanceStatus === 'suspended'

  return (
    <div className="grid grid-cols-1 lg:grid-cols-4 gap-6 items-stretch animate-in fade-in duration-300">
      {/* Left Column: Table List Sidebar */}
      <Card className="lg:col-span-1 flex flex-col overflow-hidden border-none shadow-xl bg-card/95 ring-1 ring-white/5 p-4 gap-3">
        <div className="flex items-center justify-between px-2 pt-1 border-b border-border/40 pb-2">
          <span className="text-[10px] font-black uppercase tracking-wider text-muted-foreground">{t('databaseStudio.tables.sidebarTitle')} ({schemaData.length} tables)</span>
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
                    {table.rows} rows
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
                const cols = schemaData.find(tb => tb.name === selectedTable)?.columns || []
                const initialData: Record<string, string | number | boolean | null> = {}
                cols.forEach((c: SchemaColumn) => {
                  if (c.key === 'PRI') return

                  const typeLower = c.type.toLowerCase()
                  const isDateType = typeLower.includes('date') || typeLower.includes('time') || typeLower.includes('timestamp')
                  
                  if (isDateType) {
                    const isDynamicDefault = typeof c.default === 'string' && (
                      c.default.toLowerCase().includes('current_timestamp') ||
                      c.default.toLowerCase().includes('now()') ||
                      c.default.toLowerCase().includes('uuid')
                    )
                    
                    if (isDynamicDefault) {
                      initialData[c.name] = ''
                    } else if (c.default) {
                      if (typeLower.includes('timestamp') || typeLower.includes('datetime')) {
                        initialData[c.name] = formatDatetimeLocal(c.default)
                      } else if (typeLower.includes('date')) {
                        initialData[c.name] = formatDate(c.default)
                      } else {
                        initialData[c.name] = String(c.default)
                      }
                    } else {
                      initialData[c.name] = ''
                    }
                  } else {
                    initialData[c.name] = c.default ?? ''
                  }
                })
                setInsertFormData(initialData)
                setShowRowInsertPreview(false)
                setRowInsertPreviewSql('')
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
                {schemaData.find(tb => tb.name === selectedTable)?.rows ?? tableTotal}
              </span>
            </div>
            <div className="p-3.5 bg-muted/20 border border-border/40 rounded-xl flex flex-col hover:border-primary/20 transition-all">
              <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.tables.stats.cols')}</span>
              <span className="text-lg font-extrabold text-foreground mt-0.5">
                {schemaData.find(tb => tb.name === selectedTable)?.columns?.length || 0}
              </span>
            </div>
            <div className="p-3.5 bg-muted/20 border border-border/40 rounded-xl flex flex-col hover:border-primary/20 transition-all">
              <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.tables.stats.size')}</span>
              <span className="text-lg font-extrabold text-foreground mt-0.5">
                {schemaData.find(tb => tb.name === selectedTable)?.size || '0.00 KB'}
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
                    tableData.rows.map((row: Record<string, unknown>, idx: number) => {
                      const pkColumn = tableData.columns.find((c: string) => 
                        c.toLowerCase() === 'id' || c.toLowerCase() === 'uid' || c.toLowerCase() === 'uuid'
                      ) || tableData.columns[0]

                      return (
                        <tr key={idx} className="border-b border-border/40 hover:bg-muted/15">
                          <td className="py-3.5 px-4 text-center shrink-0">
                            <DropdownMenu>
                              <DropdownMenuTrigger>
                                <Button variant="ghost" size="icon" className="h-8 w-8 hover:bg-muted/50 cursor-pointer" style={{ cursor: 'pointer' }}>
                                  <MoreHorizontal className="w-4 h-4" />
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end" className="w-32">
                                <DropdownMenuItem onClick={() => openEditRowModal(row)}>
                                  <Pencil />
                                  {t('common.edit')}
                                </DropdownMenuItem>
                                <DropdownMenuItem onClick={() => handleDeleteRow(row, pkColumn)} variant="destructive">
                                  <Trash2 />
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
                {showRowInsertPreview ? t('databaseStudio.tables.editModal.previewTitle') : t('databaseStudio.tables.insertModal.title')}
              </DialogTitle>
              <DialogDescription className="text-xs text-muted-foreground">
                {showRowInsertPreview ? (
                  <span>
                    {t('databaseStudio.tables.editModal.previewDesc')} — <span className="font-mono text-primary font-semibold">{selectedTable}</span>
                  </span>
                ) : (
                  t('databaseStudio.tables.insertModal.desc')
                )}
              </DialogDescription>
            </DialogHeader>

            {!showRowInsertPreview ? (
              <form onSubmit={handleInsertRowSubmit} className="space-y-4 pt-3">
                <div className="space-y-3 max-h-[50vh] overflow-y-auto pr-1">
                  {(tableData?.columns || []).map((col: string) => {
                    const schemaTable = schemaData.find(tb => tb.name === selectedTable)
                    const colDetail = schemaTable?.columns?.find((c: SchemaColumn) => c.name === col)
                    const isPK = colDetail?.key === 'PRI' || colDetail?.extra?.toLowerCase().includes('auto_increment')
                    const isNullable = colDetail?.nullable === 'YES' || colDetail?.null === 'YES' || colDetail?.nullable === true
                    const typeLower = (colDetail?.type || 'varchar').toLowerCase()
                    
                    return (
                      <div key={col} className="space-y-1.5">
                        <div className="flex items-center justify-between">
                          <Label htmlFor={`insert_${col}`} className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground flex items-center gap-1">
                            {col}
                            <span className="font-mono text-[9px] text-muted-foreground/50 lowercase">({colDetail?.type || 'unknown'})</span>
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
                            id={`insert_${col}`}
                            disabled
                            value={String(insertFormData[col] || '')}
                            placeholder="Auto-incrementing ID"
                            className="h-10 rounded-xl bg-muted/40 border-border/40 font-mono text-xs cursor-not-allowed"
                          />
                        ) : typeLower.includes('bool') || typeLower.includes('tinyint(1)') ? (
                          <Select
                            value={String(insertFormData[col] ?? '')}
                            onValueChange={(val) => setInsertFormData(prev => ({ ...prev, [col]: val }))}
                          >
                            <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold text-left justify-between">
                              <SelectValue placeholder={t('databaseStudio.tables.booleanSelect') || undefined} />
                            </SelectTrigger>
                            <SelectContent align="start" alignItemWithTrigger={false} className="min-w-[var(--radix-select-trigger-width)] p-1 bg-popover/98 backdrop-blur-lg border border-border/80 rounded-xl shadow-2xl max-h-72">
                              <SelectItem value="true" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">
                                {t('databaseStudio.tables.booleanTrue')}
                              </SelectItem>
                              <SelectItem value="false" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">
                                {t('databaseStudio.tables.booleanFalse')}
                              </SelectItem>
                            </SelectContent>
                          </Select>
                        ) : typeLower.includes('timestamp') || typeLower.includes('datetime') || typeLower.includes('date') ? (
                          <div className="relative flex items-center w-full">
                            <input
                              id={`insert_${col}`}
                              key={selectedTable + '_' + col}
                              type={typeLower.includes('date') && !typeLower.includes('time') ? "date" : "datetime-local"}
                              value={String(insertFormData[col] || '')}
                              onChange={(e) => {
                                const val = e.target.value
                                const isBadInput = e.target.validity?.badInput
                                if (val === '' && isBadInput) return
                                setInsertFormData(prev => ({ ...prev, [col]: val }))
                              }}
                              required={!isNullable}
                              className="w-full h-10 pl-3 pr-10 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold outline-none focus:border-primary/50 [&::-webkit-calendar-picker-indicator]:cursor-pointer"
                            />
                            <button
                              type="button"
                              onClick={() => {
                                const now = new Date()
                                const isDateOnly = typeLower.includes('date') && !typeLower.includes('time')
                                const val = isDateOnly ? toLocalDateString(now) : toLocalISOString(now)
                                setInsertFormData(prev => ({ ...prev, [col]: val }))
                              }}
                              className="absolute right-3 flex items-center justify-center p-1 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors cursor-pointer"
                              title="Set to Current Time"
                              style={{ cursor: 'pointer' }}
                            >
                              <Clock className="w-3.5 h-3.5" />
                            </button>
                          </div>
                        ) : (
                          <Input
                            id={`insert_${col}`}
                            value={String(insertFormData[col] || '')}
                            onChange={(e) => setInsertFormData(prev => ({ ...prev, [col]: e.target.value }))}
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
            ) : (
              <div className="space-y-4 pt-3">
                <div className="relative bg-muted/30 border border-border/60 rounded-xl p-3.5 font-mono text-xs text-foreground/80 whitespace-pre-wrap select-all max-h-[220px] overflow-y-auto leading-relaxed scrollbar-thin pr-12">
                  {rowInsertPreviewSql}
                  <button
                    type="button"
                    onClick={() => handleCopySql(rowInsertPreviewSql)}
                    className="absolute right-2.5 top-2.5 p-1.5 rounded-lg bg-background border border-border/40 text-muted-foreground hover:text-foreground transition-all duration-200 cursor-pointer shadow-sm"
                    title={t('common.copy')}
                    style={{ cursor: 'pointer' }}
                  >
                    {copiedSql ? <Check className="w-3.5 h-3.5 text-green-500" /> : <Copy className="w-3.5 h-3.5" />}
                  </button>
                </div>

                <div className="flex gap-2.5 pt-2 border-t border-border/40">
                  <Button onClick={handleInsertRowCommit} disabled={isActionLoading} className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {isActionLoading ? t('common.executing') : t('databaseStudio.tables.editModal.commitBtn')}
                  </Button>
                  <Button type="button" onClick={() => setShowRowInsertPreview(false)} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {t('databaseStudio.tables.editModal.backBtn')}
                  </Button>
                </div>
              </div>
            )}
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
                  {(schemaData.find(tb => tb.name === selectedTable)?.columns || []).map((col: SchemaColumn) => {
                    const pkColumn = tableData?.columns.find((c: string) => 
                      c.toLowerCase() === 'id' || c.toLowerCase() === 'uid' || c.toLowerCase() === 'uuid'
                    ) || tableData?.columns[0]
                    const isPK = col.name === pkColumn
                    const isNullable = col.nullable === 'YES' || col.nullable === true
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
                            value={String(editFormData[col.name] || '')}
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
                            <SelectContent align="start" alignItemWithTrigger={false} className="min-w-[var(--radix-select-trigger-width)] p-1 bg-popover/98 backdrop-blur-lg border border-border/80 rounded-xl shadow-2xl max-h-72">
                              <SelectItem value="true" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">
                                {t('databaseStudio.tables.booleanTrue')}
                              </SelectItem>
                              <SelectItem value="false" className="py-2 px-3 pl-8 text-xs font-medium cursor-pointer">
                                {t('databaseStudio.tables.booleanFalse')}
                              </SelectItem>
                            </SelectContent>
                          </Select>
                        ) : typeLower.includes('timestamp') || typeLower.includes('datetime') || typeLower.includes('date') ? (
                          <div className="relative flex items-center w-full">
                            <input
                              id={`edit_${col.name}`}
                              key={editingRow ? `${editingRow[pkColumn ?? col.name]}_${col.name}` : col.name}
                              type={typeLower.includes('date') && !typeLower.includes('time') ? "date" : "datetime-local"}
                              value={String(editFormData[col.name] || '')}
                              onChange={(e) => {
                                const val = e.target.value
                                const isBadInput = e.target.validity?.badInput
                                if (val === '' && isBadInput) return
                                setEditFormData(prev => ({ ...prev, [col.name]: val }))
                              }}
                              required={!isNullable}
                              className="w-full h-10 pl-3 pr-10 rounded-xl border border-border/70 bg-background/50 hover:bg-background/80 text-xs font-semibold outline-none focus:border-primary/50 [&::-webkit-calendar-picker-indicator]:cursor-pointer"
                            />
                            <button
                              type="button"
                              onClick={() => {
                                const now = new Date()
                                const isDateOnly = typeLower.includes('date') && !typeLower.includes('time')
                                const val = isDateOnly ? toLocalDateString(now) : toLocalISOString(now)
                                setEditFormData(prev => ({ ...prev, [col.name]: val }))
                              }}
                              className="absolute right-3 flex items-center justify-center p-1 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors cursor-pointer"
                              title="Set to Current Time"
                              style={{ cursor: 'pointer' }}
                            >
                              <Clock className="w-3.5 h-3.5" />
                            </button>
                          </div>
                        ) : (
                          <Input
                            id={`edit_${col.name}`}
                            value={String(editFormData[col.name] || '')}
                            onChange={(e) => setEditFormData(prev => ({ ...prev, [col.name]: e.target.value }))}
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
                    {t('databaseStudio.tables.editModal.submit')}
                  </Button>
                  <Button type="button" onClick={() => setShowEditModal(false)} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {t('common.cancel')}
                  </Button>
                </div>
              </form>
            ) : (
              <div className="space-y-4 pt-3">
                <div className="relative bg-muted/30 border border-border/60 rounded-xl p-3.5 font-mono text-xs text-foreground/80 whitespace-pre-wrap select-all max-h-[220px] overflow-y-auto leading-relaxed scrollbar-thin pr-12">
                  {rowEditPreviewSql}
                  <button
                    type="button"
                    onClick={() => handleCopySql(rowEditPreviewSql)}
                    className="absolute right-2.5 top-2.5 p-1.5 rounded-lg bg-background border border-border/40 text-muted-foreground hover:text-foreground transition-all duration-200 cursor-pointer shadow-sm"
                    title={t('common.copy')}
                    style={{ cursor: 'pointer' }}
                  >
                    {copiedSql ? <Check className="w-3.5 h-3.5 text-green-500" /> : <Copy className="w-3.5 h-3.5" />}
                  </button>
                </div>

                <div className="flex gap-2.5 pt-2 border-t border-border/40">
                  <Button onClick={handleEditRowSubmit} disabled={isActionLoading} className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {isActionLoading ? t('common.executing') : t('databaseStudio.tables.editModal.commitBtn')}
                  </Button>
                  <Button type="button" onClick={() => setShowRowEditPreview(false)} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {t('databaseStudio.tables.editModal.backBtn')}
                  </Button>
                </div>
              </div>
            )}
          </DialogContent>
        </Dialog>
      )}
    </div>
  )
}
