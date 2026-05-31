import React, { useState, useEffect } from 'react'
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
  Info
} from 'lucide-react'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
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
import { parseDbType } from './utils'

interface ForeignKeyDetail {
  column_name: string;
  target_table: string;
  target_column: string;
  on_delete?: string;
}

interface SchemaTableWithFks {
  name: string;
  columns: SchemaColumn[];
  rows?: number;
  size?: string;
  foreign_keys?: ForeignKeyDetail[];
}

interface DesignerPayload {
  action: string | null;
  table_name: string;
  column?: {
    name: string;
    type: string;
    length?: number;
    nullable?: boolean;
    default_value?: string | number | boolean | null;
    primary_key?: boolean;
    unique?: boolean;
    comment?: string | null;
    foreign_key?: boolean;
    fk_table?: string;
    fk_column?: string;
    fk_on_delete?: string;
  };
  new_name?: string;
  index_name?: string;
  index_columns?: string[];
}

export function StudioStructureTab() {
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

  const typedSchemaData = schemaData as SchemaTableWithFks[]

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
  const [structureSearch, setStructureSearch] = useState('')

  // Visual Designer states
  const [designerAction, setDesignerAction] = useState<'create_table' | 'add_column' | 'create_index' | 'modify_column' | null>(null)
  
  // Create Table Form
  const [newTableName, setNewTableName] = useState('')

  // Add Column Form
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
  
  // Create Index Form
  const [indexName, setIndexName] = useState('')
  const [indexCols, setIndexCols] = useState<string[]>([])

  // Modify Column Form
  const [editingCol, setEditingCol] = useState<SchemaColumn | null>(null)
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

  // Set default selected table on schemaData load
  useEffect(() => {
    if (typedSchemaData.length > 0 && !selectedTable) {
      setSelectedTable(typedSchemaData[0].name)
    }
  }, [typedSchemaData, selectedTable])

  const filteredTables = typedSchemaData.filter(tb => 
    tb.name.toLowerCase().includes(structureSearch.toLowerCase())
  )

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
        } catch (error) {
          const err = error as { response?: { data?: { error?: string } } }
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
          }

          loadStudioData()
        } catch (error) {
          const err = error as { response?: { data?: { error?: string } } }
          toast.error(err.response?.data?.error || t('databaseStudio.errors.designerActionFailed'))
        } finally {
          setIsActionLoading(false)
        }
      }
    })
  }

  const openModifyColumnModal = (tableName: string, col: SchemaColumn) => {
    setSelectedTable(tableName)
    setEditingCol(col)
    setEditColNewName(col.name)
    
    const table = typedSchemaData.find(tb => tb.name === tableName)
    const tableFks = table?.foreign_keys || []
    const fk = tableFks.find((f) => f.column_name === col.name)
    
    const parsed = parseDbType(col.type)
    setEditColType(parsed.type)
    setEditColLength(parsed.length)
    setEditColNullable(col.nullable === 'YES' || col.nullable === true)
    setEditColDefault(col.default !== null && col.default !== undefined ? String(col.default) : '')
    setEditColPk(col.key === 'PRI')
    setEditColUnique(col.key === 'UNI')
    setEditColComment(col.extra || '')
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
    } else if (editColType === 'bigint') {
      dbType = 'BIGINT'
    } else if (editColType === 'boolean') {
      dbType = isPostgres ? 'BOOLEAN' : 'TINYINT(1)'
    } else if (editColType === 'decimal') {
      dbType = 'DECIMAL(10,2)'
    } else if (editColType === 'double') {
      dbType = isPostgres ? 'DOUBLE PRECISION' : 'DOUBLE'
    } else if (editColType === 'json') {
      dbType = isPostgres ? 'JSONB' : 'JSON'
    } else if (editColType === 'uuid') {
      dbType = isPostgres ? 'UUID' : 'VARCHAR(36)'
    } else if (editColType === 'date') {
      dbType = 'DATE'
    } else if (editColType === 'timestamp') {
      dbType = isPostgres ? 'TIMESTAMP WITH TIME ZONE' : 'DATETIME'
    } else if (editColType === 'longtext') {
      dbType = isPostgres ? 'TEXT' : 'LONGTEXT'
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

    const payload: DesignerPayload = {
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
      resetModifyColumnForm()
      loadStudioData()
    } catch (error) {
      const err = error as { response?: { data?: { error?: string } } }
      toast.error(err.response?.data?.error || t('databaseStudio.errors.designerActionFailed'))
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleDesignerAction = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!id) return
    
    const payload: DesignerPayload = {
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
      if (designerAction === 'create_table') {
        setSelectedTable(newTableName)
        setNewTableName('')
      } else if (designerAction === 'add_column') {
        resetAddColumnForm()
      } else if (designerAction === 'create_index') {
        setIndexName('')
        setIndexCols([])
      }
      loadStudioData()
    } catch (error) {
      const err = error as { response?: { data?: { error?: string } } }
      toast.error(err.response?.data?.error || t('databaseStudio.errors.designerActionFailed'))
    } finally {
      setIsActionLoading(false)
    }
  }

  const activeTableData = typedSchemaData.find(tb => tb.name === selectedTable)
  const isSuspended = dbOverview?.status === 'suspended'

  return (
    <div className="grid grid-cols-1 lg:grid-cols-4 gap-6 items-stretch animate-in fade-in duration-300">
      {/* Left Sidebar: Tables navigation */}
      <Card className="lg:col-span-1 flex flex-col overflow-hidden border-none shadow-xl bg-card/95 ring-1 ring-white/5 p-4 gap-3">
        <div className="flex items-center justify-between px-2 pt-1 border-b border-border/40 pb-2">
          <span className="text-[10px] font-black uppercase tracking-wider text-muted-foreground">{t('databaseStudio.tables.sidebarTitle')} ({schemaData.length} tables)</span>
          {!isSuspended && (
            <button
              onClick={() => setDesignerAction('create_table')}
              className="flex items-center justify-center w-6 h-6 rounded-md bg-white border border-border/10 text-neutral-950 hover:bg-neutral-100 transition-colors shadow-sm cursor-pointer"
              title={t('databaseStudio.tables.addTable')}
              style={{ cursor: 'pointer' }}
            >
              <Plus className="w-3.5 h-3.5" />
            </button>
          )}
        </div>
        
        {!isSuspended && schemaData.length > 0 && (
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground/60" />
            <Input
              placeholder={t('databaseStudio.tables.searchPlaceholder')}
              value={structureSearch}
              onChange={(e) => setStructureSearch(e.target.value)}
              className="pl-9 h-10 text-xs font-semibold rounded-xl bg-background/50 border-border/70 hover:border-primary/30"
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
                onClick={() => setSelectedTable(table.name)}
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
                     "text-[9px] font-mono px-1.5 py-0.5 rounded-md shrink-0",
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

      {/* Right Column: Visual DDL Schema Designer */}
      <Card className="lg:col-span-3 p-6 flex flex-col overflow-hidden">
        <div className="flex items-start sm:items-center justify-between gap-4 border-b pb-4 mb-5">
          <div className="flex items-center gap-3">
            <Table className="w-5 h-5 text-primary" />
            <div>
              <h3 className="font-extrabold text-base">{t('databaseStudio.structure.title')}</h3>
              <p className="text-muted-foreground text-xs">{t('databaseStudio.structure.subtitle')}</p>
            </div>
          </div>
          {selectedTable && !isSuspended && (
            <div className="flex items-center gap-2">
              <Button
                onClick={() => setDesignerAction('add_column')}
                variant="outline"
                className="font-bold gap-1.5 h-10 px-3.5 rounded-xl border border-border/80 hover:bg-muted text-xs cursor-pointer"
                style={{ cursor: 'pointer' }}
              >
                <Plus className="w-4 h-4 text-primary" />
                {t('databaseStudio.structure.addColumn')}
              </Button>
              <Button
                onClick={() => handleDeleteTable(selectedTable)}
                variant="destructive"
                className="font-bold gap-1.5 h-10 px-3.5 rounded-xl text-xs cursor-pointer"
                style={{ cursor: 'pointer' }}
              >
                <Trash2 className="w-4 h-4" />
                {t('databaseStudio.structure.actions.dropTable')}
              </Button>
            </div>
          )}
        </div>

        {isSuspended ? (
          <div className="py-12 text-center text-muted-foreground text-sm font-semibold uppercase tracking-wide">
            {t('databaseStudio.dashboard.suspendedWarning')}
          </div>
        ) : !selectedTable ? (
          <div className="py-12 border border-dashed rounded-xl flex flex-col items-center justify-center text-center gap-3 bg-muted/5">
            <Table className="w-8 h-8 text-muted-foreground" />
            <div className="space-y-1">
              <h4 className="font-bold text-sm">{t('databaseStudio.structure.noSchemaObjects')}</h4>
              <p className="text-xs text-muted-foreground">{t('databaseStudio.structure.noSchemaObjectsDesc')}</p>
              <Button onClick={() => setDesignerAction('create_table')} className="mt-2 font-bold cursor-pointer" style={{ cursor: 'pointer' }}>
                <Plus className="w-4 h-4 mr-1" />
                {t('databaseStudio.tables.addTable')}
              </Button>
            </div>
          </div>
        ) : activeTableData ? (
          <div className="space-y-6 flex-1 flex flex-col min-h-0">
            <div className="flex items-center justify-between text-xs font-semibold px-1">
              <span className="text-muted-foreground uppercase tracking-wider">{selectedTable} Schema Definition</span>
              <span className="font-mono bg-muted/30 border border-border/60 px-2 py-0.5 rounded-md text-[10px]">
                {t('databaseStudio.structure.columnsCount', { count: activeTableData.columns?.length || 0 })}
              </span>
            </div>

            <div className="overflow-x-auto border border-border/80 rounded-xl bg-background/30 max-h-[350px] flex-1">
              <table className="w-full text-left border-collapse text-xs font-medium">
                <thead>
                  <tr className="bg-muted border-b border-border/80 text-[10px] font-bold uppercase tracking-widest text-muted-foreground sticky top-0 z-10">
                    <th className="py-3.5 px-4 w-12 text-center bg-muted">{t('databaseStudio.tables.actionHeader')}</th>
                    <th className="py-3.5 px-4 bg-muted">{t('databaseStudio.structure.nameHeader')}</th>
                    <th className="py-3.5 px-4 bg-muted">{t('databaseStudio.structure.typeHeader')}</th>
                    <th className="py-3.5 px-4 bg-muted">{t('databaseStudio.structure.keyHeader')}</th>
                    <th className="py-3.5 px-4 bg-muted">{t('databaseStudio.structure.constraintsHeader')}</th>
                    <th className="py-3.5 px-4 bg-muted">{t('databaseStudio.structure.defaultHeader')}</th>
                  </tr>
                </thead>
                <tbody>
                  {activeTableData.columns?.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="py-8 text-center text-muted-foreground italic font-semibold">
                        This table has no columns defined.
                      </td>
                    </tr>
                  ) : (
                    activeTableData.columns.map((col: SchemaColumn) => {
                      const isPri = col.key === 'PRI'
                      const isUni = col.key === 'UNI'
                      const hasFk = activeTableData.foreign_keys?.some((f) => f.column_name === col.name)
                      const fkDetail = activeTableData.foreign_keys?.find((f) => f.column_name === col.name)

                      return (
                        <tr key={col.name} className="border-b border-border/40 hover:bg-muted/15">
                          <td className="py-3 px-4 text-center shrink-0">
                            <DropdownMenu>
                              <DropdownMenuTrigger>
                                <Button variant="ghost" size="icon" className="h-8 w-8 hover:bg-muted/50 cursor-pointer" style={{ cursor: 'pointer' }}>
                                  <MoreHorizontal className="w-4 h-4" />
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end" className="w-36">
                                <DropdownMenuItem onClick={() => openModifyColumnModal(selectedTable, col)}>
                                  <Pencil />
                                  {t('databaseStudio.structure.actions.modifyColumn')}
                                </DropdownMenuItem>
                                {!isPri && (
                                  <DropdownMenuItem onClick={() => handleDropColumn(selectedTable, col.name)} variant="destructive">
                                    <Trash2 />
                                    {t('databaseStudio.structure.actions.dropColumn')}
                                  </DropdownMenuItem>
                                )}
                              </DropdownMenuContent>
                            </DropdownMenu>
                          </td>
                          <td className="py-3 px-4 font-mono font-bold text-foreground">{col.name}</td>
                          <td className="py-3 px-4 font-mono text-muted-foreground">{col.type}</td>
                          <td className="py-3 px-4 font-mono">
                            <div className="flex items-center gap-1.5 flex-wrap">
                              {isPri && (
                                <span className="px-1.5 py-0.5 rounded bg-amber-500/10 text-amber-500 border border-amber-500/20 text-[9px] font-black uppercase" title={t('databaseStudio.structure.tooltips.primaryKey')}>
                                  PRI
                                </span>
                              )}
                              {isUni && (
                                <span className="px-1.5 py-0.5 rounded bg-primary/10 text-primary border border-primary/20 text-[9px] font-black uppercase" title={t('databaseStudio.structure.tooltips.unique')}>
                                  UNI
                                </span>
                              )}
                              {hasFk && fkDetail && (
                                <span className="px-1.5 py-0.5 rounded bg-purple-500/10 text-purple-500 border border-purple-500/20 text-[9px] font-black uppercase" title={t('databaseStudio.structure.tooltips.foreignKey', { table: fkDetail.target_table, column: fkDetail.target_column })}>
                                  FK
                                </span>
                              )}
                            </div>
                          </td>
                          <td className="py-3 px-4 font-mono">
                            {col.nullable === 'YES' || col.nullable === true ? (
                              <span className="text-[10px] text-muted-foreground">{t('databaseStudio.structure.nullable')}</span>
                            ) : (
                              <span className="text-[10px] text-foreground font-semibold">{t('databaseStudio.structure.notNullable')}</span>
                            )}
                          </td>
                          <td className="py-3 px-4 font-mono text-muted-foreground/80">
                            {col.default !== null && col.default !== undefined ? String(col.default) : <span className="text-muted-foreground/20 italic">NULL</span>}
                          </td>
                        </tr>
                      )
                    })
                  )}
                </tbody>
              </table>
            </div>
          </div>
        ) : (
          <div className="py-10 text-center text-muted-foreground flex-1 flex items-center justify-center">Loading Table Schema...</div>
        )}
      </Card>

      {/* Visual Table Designer Modals */}
      {designerAction === 'create_table' && (
        <Dialog open={designerAction === 'create_table'} onOpenChange={(open: boolean) => !open && setDesignerAction(null)}>
          <DialogContent className="sm:max-w-md bg-card border border-border/80 rounded-xl shadow-2xl">
            <DialogHeader className="pb-2 border-b border-border/40">
              <DialogTitle className="text-lg font-extrabold flex items-center gap-2 text-foreground/90">
                <PlusCircle className="w-5 h-5 text-primary" />
                {t('databaseStudio.structure.createTableDialog.title')}
              </DialogTitle>
              <DialogDescription className="text-xs text-muted-foreground">
                {t('databaseStudio.structure.createTableDialog.desc')}
              </DialogDescription>
            </DialogHeader>
            <form onSubmit={handleDesignerAction} className="space-y-4 pt-3">
              <div className="space-y-1.5">
                <Label htmlFor="create_tbl_name" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                  {t('databaseStudio.structure.createTableDialog.tableName')}
                </Label>
                <Input
                  id="create_tbl_name"
                  value={newTableName}
                  onChange={(e) => setNewTableName(e.target.value)}
                  placeholder={t('databaseStudio.structure.createTableDialog.tableNamePlaceholder')}
                  required
                  className="h-10 rounded-xl bg-background/50 text-xs"
                />
              </div>

              <div className="bg-amber-500/5 border border-amber-500/10 rounded-xl p-3 flex items-start gap-2 text-[10px] text-amber-500/80 leading-normal">
                <Info className="w-3.5 h-3.5 shrink-0 mt-0.5" />
                <span>{t('databaseStudio.structure.createTableDialog.autoDesignWarning')}</span>
              </div>

              <div className="flex gap-2.5 pt-2 border-t border-border/40">
                <Button type="submit" disabled={isActionLoading || !newTableName.trim()} className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
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

      {/* Add Column Dialog */}
      {designerAction === 'add_column' && (
        <Dialog open={designerAction === 'add_column'} onOpenChange={(open: boolean) => !open && resetAddColumnForm()}>
          <DialogContent className="sm:max-w-md bg-card border border-border/80 rounded-xl shadow-2xl">
            <DialogHeader className="pb-2 border-b border-border/40">
              <DialogTitle className="text-lg font-extrabold flex items-center gap-2 text-foreground/90">
                <PlusCircle className="w-5 h-5 text-primary" />
                {t('databaseStudio.structure.addColumnDialog.title')}
              </DialogTitle>
              <DialogDescription className="text-xs text-muted-foreground">
                {t('databaseStudio.structure.addColumnDialog.desc')} — <span className="font-mono text-primary font-semibold">{selectedTable}</span>
              </DialogDescription>
            </DialogHeader>
            <form onSubmit={handleDesignerAction} className="space-y-4 pt-3">
              <div className="space-y-3.5 max-h-[50vh] overflow-y-auto pr-1">
                <div className="space-y-1.5">
                  <Label htmlFor="add_col_name" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                    {t('databaseStudio.structure.createTableDialog.columnName')}
                  </Label>
                  <Input
                    id="add_col_name"
                    value={newColName}
                    onChange={(e) => setNewColName(e.target.value)}
                    placeholder={t('databaseStudio.structure.addColumnDialog.columnNamePlaceholder')}
                    required
                    className="h-10 rounded-xl bg-background/50 text-xs"
                  />
                </div>

                <div className="grid grid-cols-2 gap-4 items-start">
                  <div className="space-y-1.5">
                    <Label htmlFor="add_col_type" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                      {t('databaseStudio.structure.typeHeader')}
                    </Label>
                    <Select value={newColType} onValueChange={(val) => val && setNewColType(val)}>
                      <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 text-xs font-semibold text-left data-[size=default]:h-10 data-[size=default]:py-0">
                        <SelectValue>
                          {(value) => {
                            if (!value) return '';
                            return t(`databaseStudio.structure.types.${String(value)}.label`) || String(value).toUpperCase();
                          }}
                        </SelectValue>
                      </SelectTrigger>
                      <SelectContent align="start" alignItemWithTrigger={false} className="w-[320px] p-1 bg-popover border border-border/80 rounded-xl shadow-2xl max-h-72">
                        <SelectItem value="varchar" className="py-2 pl-3 cursor-pointer">
                          <div className="flex flex-col gap-0.5 text-left">
                            <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.varchar.label')}</span>
                            <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.varchar.desc')}</span>
                          </div>
                        </SelectItem>
                        <SelectItem value="integer" className="py-2 pl-3 cursor-pointer">
                          <div className="flex flex-col gap-0.5 text-left">
                            <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.integer.label')}</span>
                            <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.integer.desc')}</span>
                          </div>
                        </SelectItem>
                        <SelectItem value="bigint" className="py-2 pl-3 cursor-pointer">
                          <div className="flex flex-col gap-0.5 text-left">
                            <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.bigint.label')}</span>
                            <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.bigint.desc')}</span>
                          </div>
                        </SelectItem>
                        <SelectItem value="boolean" className="py-2 pl-3 cursor-pointer">
                          <div className="flex flex-col gap-0.5 text-left">
                            <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.boolean.label')}</span>
                            <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.boolean.desc')}</span>
                          </div>
                        </SelectItem>
                        <SelectItem value="text" className="py-2 pl-3 cursor-pointer">
                          <div className="flex flex-col gap-0.5 text-left">
                            <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.text.label')}</span>
                            <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.text.desc')}</span>
                          </div>
                        </SelectItem>
                        <SelectItem value="longtext" className="py-2 pl-3 cursor-pointer">
                          <div className="flex flex-col gap-0.5 text-left">
                            <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.longtext.label')}</span>
                            <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.longtext.desc')}</span>
                          </div>
                        </SelectItem>
                        <SelectItem value="decimal" className="py-2 pl-3 cursor-pointer">
                          <div className="flex flex-col gap-0.5 text-left">
                            <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.decimal.label')}</span>
                            <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.decimal.desc')}</span>
                          </div>
                        </SelectItem>
                        <SelectItem value="double" className="py-2 pl-3 cursor-pointer">
                          <div className="flex flex-col gap-0.5 text-left">
                            <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.double.label')}</span>
                            <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.double.desc')}</span>
                          </div>
                        </SelectItem>
                        <SelectItem value="json" className="py-2 pl-3 cursor-pointer">
                          <div className="flex flex-col gap-0.5 text-left">
                            <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.json.label')}</span>
                            <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.json.desc')}</span>
                          </div>
                        </SelectItem>
                        <SelectItem value="uuid" className="py-2 pl-3 cursor-pointer">
                          <div className="flex flex-col gap-0.5 text-left">
                            <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.uuid.label')}</span>
                            <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.uuid.desc')}</span>
                          </div>
                        </SelectItem>
                        <SelectItem value="date" className="py-2 pl-3 cursor-pointer">
                          <div className="flex flex-col gap-0.5 text-left">
                            <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.date.label')}</span>
                            <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.date.desc')}</span>
                          </div>
                        </SelectItem>
                        <SelectItem value="timestamp" className="py-2 pl-3 cursor-pointer">
                          <div className="flex flex-col gap-0.5 text-left">
                            <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.timestamp.label')}</span>
                            <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.timestamp.desc')}</span>
                          </div>
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="space-y-1.5">
                    <Label htmlFor="add_col_len" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                      {t('databaseStudio.structure.addColumnDialog.lengthLabel')}
                    </Label>
                    {newColType === 'varchar' ? (
                      <Input
                        id="add_col_len"
                        type="number"
                        value={newColLength}
                        onChange={(e) => setNewColLength(e.target.value)}
                        required
                        className="h-10 rounded-xl bg-background/50 text-xs"
                      />
                    ) : (
                      <Input
                        id="add_col_len"
                        value="-"
                        disabled
                        className="h-10 rounded-xl bg-background/30 text-xs text-muted-foreground/60"
                      />
                    )}
                  </div>
                </div>

                <div className="border border-border/60 rounded-xl p-4 bg-muted/5 space-y-4">
                  <span className="text-[10px] font-black uppercase tracking-wider text-muted-foreground border-b pb-1.5 flex items-center">
                    {t('databaseStudio.structure.designer.configHeader')}
                  </span>

                  <div className="flex flex-col gap-3 text-xs">
                    <label className="flex items-start gap-2.5 font-medium cursor-pointer select-none">
                      <Checkbox
                        checked={newColNullable}
                        onCheckedChange={(checked) => setNewColNullable(Boolean(checked))}
                        className="mt-0.5 h-4 w-4 rounded-[5px] border-border/80 data-[state=checked]:border-primary data-[state=checked]:bg-primary/90 data-[state=checked]:text-primary-foreground shadow-sm"
                      />
                      <div className="flex flex-col">
                        <span>{t('databaseStudio.structure.createTableDialog.nullableLabel')}</span>
                        <span className="text-[10px] text-muted-foreground font-normal">{t('databaseStudio.structure.designer.nullableDesc')}</span>
                      </div>
                    </label>

                    <label className="flex items-start gap-2.5 font-medium cursor-pointer select-none">
                      <Checkbox
                        checked={newColUnique}
                        onCheckedChange={(checked) => setNewColUnique(Boolean(checked))}
                        className="mt-0.5 h-4 w-4 rounded-[5px] border-border/80 data-[state=checked]:border-primary data-[state=checked]:bg-primary/90 data-[state=checked]:text-primary-foreground shadow-sm"
                      />
                      <div className="flex flex-col">
                        <span>{t('databaseStudio.structure.uniqueConstraint')}</span>
                        <span className="text-[10px] text-muted-foreground font-normal">{t('databaseStudio.structure.designer.uniqueDesc')}</span>
                      </div>
                    </label>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="add_col_def" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                    {t('databaseStudio.structure.createTableDialog.defaultValue')}
                  </Label>
                  <Input
                    id="add_col_def"
                    value={newColDefault}
                    onChange={(e) => setNewColDefault(e.target.value)}
                    placeholder="NULL"
                    className="h-10 rounded-xl bg-background/50 text-xs"
                  />
                </div>

                <div className="space-y-1.5 font-medium border border-border/60 rounded-xl p-4 bg-muted/5 space-y-4">
                  <label className="flex items-start gap-2.5 cursor-pointer select-none">
                    <Checkbox
                      checked={newColFk}
                      onCheckedChange={(checked) => setNewColFk(Boolean(checked))}
                      className="mt-0.5 h-4 w-4 rounded-[5px] border-border/80 data-[state=checked]:border-primary data-[state=checked]:bg-primary/90 data-[state=checked]:text-primary-foreground shadow-sm"
                    />
                    <div className="flex flex-col">
                      <span>{t('databaseStudio.structure.designer.fkRelation')}</span>
                      <span className="text-[10px] text-muted-foreground font-normal">Link this column to reference another table's key</span>
                    </div>
                  </label>

                  {newColFk && (
                    <div className="space-y-3.5 pt-2 border-t border-border/40 animate-in fade-in duration-200">
                      <div className="grid grid-cols-2 gap-4">
                        <div className="space-y-1.5">
                          <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                            {t('databaseStudio.structure.designer.targetTable')}
                          </Label>
                          <Select value={newColFkTargetTable} onValueChange={(val) => val && setNewColFkTargetTable(val)}>
                            <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 text-xs font-semibold text-left">
                              <SelectValue placeholder={t('databaseStudio.structure.designer.selectTable')} />
                            </SelectTrigger>
                            <SelectContent align="start" alignItemWithTrigger={false} className="min-w-[var(--radix-select-trigger-width)] p-1 bg-popover border border-border/80 rounded-xl shadow-2xl max-h-72">
                              {typedSchemaData.map(tb => (
                                <SelectItem key={tb.name} value={tb.name} className="text-xs font-medium py-2 pl-3 cursor-pointer">{tb.name}</SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>

                        <div className="space-y-1.5">
                          <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                            {t('databaseStudio.structure.designer.targetColumn')}
                          </Label>
                          <Select value={newColFkTargetColumn} onValueChange={(val) => val && setNewColFkTargetColumn(val)}>
                            <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 text-xs font-semibold text-left">
                              <SelectValue placeholder={t('databaseStudio.structure.designer.selectColumn')} />
                            </SelectTrigger>
                            <SelectContent align="start" alignItemWithTrigger={false} className="min-w-[var(--radix-select-trigger-width)] p-1 bg-popover border border-border/80 rounded-xl shadow-2xl max-h-72">
                              {newColFkTargetTable && typedSchemaData.find(tb => tb.name === newColFkTargetTable)?.columns.map((c) => (
                                <SelectItem key={c.name} value={c.name} className="text-xs font-medium py-2 pl-3 cursor-pointer">{c.name}</SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                      </div>

                      <div className="space-y-1.5">
                        <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                          {t('databaseStudio.structure.designer.onDeleteAction')}
                        </Label>
                        <Select value={newColFkOnDelete} onValueChange={(val) => val && setNewColFkOnDelete(val)}>
                          <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 text-xs font-semibold text-left">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent align="start" alignItemWithTrigger={false} className="min-w-[var(--radix-select-trigger-width)] p-1 bg-popover border border-border/80 rounded-xl shadow-2xl max-h-72">
                            <SelectItem value="CASCADE" className="text-xs font-medium py-2 pl-3 cursor-pointer">CASCADE</SelectItem>
                            <SelectItem value="SET NULL" className="text-xs font-medium py-2 pl-3 cursor-pointer">SET NULL</SelectItem>
                            <SelectItem value="RESTRICT" className="text-xs font-medium py-2 pl-3 cursor-pointer">RESTRICT</SelectItem>
                            <SelectItem value="NO ACTION" className="text-xs font-medium py-2 pl-3 cursor-pointer">NO ACTION</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                    </div>
                  )}
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="add_col_comment" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                    {t('databaseStudio.structure.designer.commentLabel')}
                  </Label>
                  <Input
                    id="add_col_comment"
                    value={newColComment}
                    onChange={(e) => setNewColComment(e.target.value)}
                    placeholder={t('databaseStudio.structure.designer.commentPlaceholder')}
                    className="h-10 rounded-xl bg-background/50 text-xs"
                  />
                </div>
              </div>

              <div className="flex gap-2.5 pt-2 border-t border-border/40">
                <Button type="submit" disabled={isActionLoading || !newColName.trim()} className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                  {isActionLoading ? t('common.executing') : t('databaseStudio.structure.addColumnDialog.submitBtn')}
                </Button>
                <Button type="button" onClick={() => resetAddColumnForm()} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                  {t('common.cancel')}
                </Button>
              </div>
            </form>
          </DialogContent>
        </Dialog>
      )}

      {/* Modify Column Dialog */}
      {designerAction === 'modify_column' && editingCol && (
        <Dialog open={designerAction === 'modify_column'} onOpenChange={(open: boolean) => !open && resetModifyColumnForm()}>
          <DialogContent className="sm:max-w-md bg-card border border-border/80 rounded-xl shadow-2xl">
            <DialogHeader className="pb-2 border-b border-border/40">
              <DialogTitle className="text-lg font-extrabold flex items-center gap-2 text-foreground/90">
                <Pencil className="w-5 h-5 text-primary" />
                {showColModifyPreview ? t('databaseStudio.structure.modifyColumnDialog.previewTitle') : t('databaseStudio.structure.modifyColumnDialog.title')}
              </DialogTitle>
              <DialogDescription className="text-xs text-muted-foreground">
                {showColModifyPreview ? t('databaseStudio.structure.modifyColumnDialog.previewDesc') : t('databaseStudio.structure.modifyColumnDialog.desc')} — <span className="font-mono text-primary font-semibold">{selectedTable}</span>
              </DialogDescription>
            </DialogHeader>

            {!showColModifyPreview ? (
              <form onSubmit={handleModifyColumnFormSubmit} className="space-y-4 pt-3">
                <div className="space-y-3.5 max-h-[50vh] overflow-y-auto pr-1">
                  <div className="space-y-1.5">
                    <Label htmlFor="mod_col_name" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                      {t('databaseStudio.structure.modifyColumnDialog.columnNameLabel')}
                    </Label>
                    <Input
                      id="mod_col_name"
                      value={editColNewName}
                      onChange={(e) => setEditColNewName(e.target.value)}
                      required
                      className="h-10 rounded-xl bg-background/50 text-xs"
                    />
                  </div>

                  <div className="grid grid-cols-2 gap-4 items-start">
                    <div className="space-y-1.5">
                      <Label htmlFor="mod_col_type" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                        {t('databaseStudio.structure.typeHeader')}
                      </Label>
                      <Select value={editColType} onValueChange={(val) => val && setEditColType(val)}>
                        <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 text-xs font-semibold text-left data-[size=default]:h-10 data-[size=default]:py-0">
                          <SelectValue>
                            {(value) => {
                              if (!value) return '';
                              return t(`databaseStudio.structure.types.${String(value)}.label`) || String(value).toUpperCase();
                            }}
                          </SelectValue>
                        </SelectTrigger>
                        <SelectContent align="start" alignItemWithTrigger={false} className="w-[320px] p-1 bg-popover border border-border/80 rounded-xl shadow-2xl max-h-72">
                          <SelectItem value="varchar" className="py-2 pl-3 cursor-pointer">
                            <div className="flex flex-col gap-0.5 text-left">
                              <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.varchar.label')}</span>
                              <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.varchar.desc')}</span>
                            </div>
                          </SelectItem>
                          <SelectItem value="integer" className="py-2 pl-3 cursor-pointer">
                            <div className="flex flex-col gap-0.5 text-left">
                              <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.integer.label')}</span>
                              <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.integer.desc')}</span>
                            </div>
                          </SelectItem>
                          <SelectItem value="bigint" className="py-2 pl-3 cursor-pointer">
                            <div className="flex flex-col gap-0.5 text-left">
                              <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.bigint.label')}</span>
                              <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.bigint.desc')}</span>
                            </div>
                          </SelectItem>
                          <SelectItem value="boolean" className="py-2 pl-3 cursor-pointer">
                            <div className="flex flex-col gap-0.5 text-left">
                              <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.boolean.label')}</span>
                              <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.boolean.desc')}</span>
                            </div>
                          </SelectItem>
                          <SelectItem value="text" className="py-2 pl-3 cursor-pointer">
                            <div className="flex flex-col gap-0.5 text-left">
                              <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.text.label')}</span>
                              <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.text.desc')}</span>
                            </div>
                          </SelectItem>
                          <SelectItem value="longtext" className="py-2 pl-3 cursor-pointer">
                            <div className="flex flex-col gap-0.5 text-left">
                              <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.longtext.label')}</span>
                              <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.longtext.desc')}</span>
                            </div>
                          </SelectItem>
                          <SelectItem value="decimal" className="py-2 pl-3 cursor-pointer">
                            <div className="flex flex-col gap-0.5 text-left">
                              <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.decimal.label')}</span>
                              <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.decimal.desc')}</span>
                            </div>
                          </SelectItem>
                          <SelectItem value="double" className="py-2 pl-3 cursor-pointer">
                            <div className="flex flex-col gap-0.5 text-left">
                              <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.double.label')}</span>
                              <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.double.desc')}</span>
                            </div>
                          </SelectItem>
                          <SelectItem value="json" className="py-2 pl-3 cursor-pointer">
                            <div className="flex flex-col gap-0.5 text-left">
                              <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.json.label')}</span>
                              <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.json.desc')}</span>
                            </div>
                          </SelectItem>
                          <SelectItem value="uuid" className="py-2 pl-3 cursor-pointer">
                            <div className="flex flex-col gap-0.5 text-left">
                              <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.uuid.label')}</span>
                              <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.uuid.desc')}</span>
                            </div>
                          </SelectItem>
                          <SelectItem value="date" className="py-2 pl-3 cursor-pointer">
                            <div className="flex flex-col gap-0.5 text-left">
                              <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.date.label')}</span>
                              <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.date.desc')}</span>
                            </div>
                          </SelectItem>
                          <SelectItem value="timestamp" className="py-2 pl-3 cursor-pointer">
                            <div className="flex flex-col gap-0.5 text-left">
                              <span className="text-sm font-semibold leading-none">{t('databaseStudio.structure.types.timestamp.label')}</span>
                              <span className="text-[10px] leading-none text-muted-foreground">{t('databaseStudio.structure.types.timestamp.desc')}</span>
                            </div>
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    </div>

                    <div className="space-y-1.5">
                      <Label htmlFor="mod_col_len" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                        {t('databaseStudio.structure.addColumnDialog.lengthLabel')}
                      </Label>
                      {editColType === 'varchar' ? (
                        <Input
                          id="mod_col_len"
                          type="number"
                          value={editColLength}
                          onChange={(e) => setEditColLength(e.target.value)}
                          required
                          className="h-10 rounded-xl bg-background/50 text-xs"
                        />
                      ) : (
                        <Input
                          id="mod_col_len"
                          value="-"
                          disabled
                          className="h-10 rounded-xl bg-background/30 text-xs text-muted-foreground/60"
                        />
                      )}
                    </div>
                  </div>

                  <div className="border border-border/60 rounded-xl p-4 bg-muted/5 space-y-4">
                    <span className="text-[10px] font-black uppercase tracking-wider text-muted-foreground border-b pb-1.5 flex items-center">
                      {t('databaseStudio.structure.designer.configHeader')}
                    </span>
                    
                    <div className="flex flex-col gap-3 text-xs">
                      <label className="flex items-start gap-2.5 font-medium cursor-pointer select-none">
                        <Checkbox
                          checked={editColNullable}
                          onCheckedChange={(checked) => setEditColNullable(Boolean(checked))}
                          className="mt-0.5 h-4 w-4 rounded-[5px] border-border/80 data-[state=checked]:border-primary data-[state=checked]:bg-primary/90 data-[state=checked]:text-primary-foreground shadow-sm"
                        />
                        <div className="flex flex-col">
                          <span>{t('databaseStudio.structure.createTableDialog.nullableLabel')}</span>
                          <span className="text-[10px] text-muted-foreground font-normal">{t('databaseStudio.structure.designer.nullableDesc')}</span>
                        </div>
                      </label>

                      <label className="flex items-start gap-2.5 font-medium cursor-pointer select-none">
                        <Checkbox
                          checked={editColUnique}
                          onCheckedChange={(checked) => setEditColUnique(Boolean(checked))}
                          className="mt-0.5 h-4 w-4 rounded-[5px] border-border/80 data-[state=checked]:border-primary data-[state=checked]:bg-primary/90 data-[state=checked]:text-primary-foreground shadow-sm"
                        />
                        <div className="flex flex-col">
                          <span>{t('databaseStudio.structure.uniqueConstraint')}</span>
                          <span className="text-[10px] text-muted-foreground font-normal">{t('databaseStudio.structure.designer.uniqueDesc')}</span>
                        </div>
                      </label>
                    </div>
                  </div>

                  <div className="space-y-1.5">
                    <Label htmlFor="mod_col_def" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                      {t('databaseStudio.structure.createTableDialog.defaultValue')}
                    </Label>
                    <Input
                      id="mod_col_def"
                      value={editColDefault}
                      onChange={(e) => setEditColDefault(e.target.value)}
                      placeholder="NULL"
                      className="h-10 rounded-xl bg-background/50 text-xs"
                    />
                  </div>

                  <div className="space-y-1.5 font-medium border border-border/60 rounded-xl p-4 bg-muted/5 space-y-4">
                    <label className="flex items-start gap-2.5 cursor-pointer select-none">
                      <Checkbox
                        checked={editColFk}
                        onCheckedChange={(checked) => setEditColFk(Boolean(checked))}
                        className="mt-0.5 h-4 w-4 rounded-[5px] border-border/80 data-[state=checked]:border-primary data-[state=checked]:bg-primary/90 data-[state=checked]:text-primary-foreground shadow-sm"
                      />
                      <div className="flex flex-col">
                        <span>{t('databaseStudio.structure.designer.fkRelation')}</span>
                        <span className="text-[10px] text-muted-foreground font-normal">Link this column to reference another table's key</span>
                      </div>
                    </label>

                    {editColFk && (
                      <div className="space-y-3.5 pt-2 border-t border-border/40 animate-in fade-in duration-200">
                        <div className="grid grid-cols-2 gap-4">
                          <div className="space-y-1.5">
                            <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                              {t('databaseStudio.structure.designer.targetTable')}
                            </Label>
                            <Select value={editColFkTargetTable} onValueChange={(val) => val && setEditColFkTargetTable(val)}>
                              <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 text-xs font-semibold text-left">
                                <SelectValue placeholder={t('databaseStudio.structure.designer.selectTable')} />
                              </SelectTrigger>
                              <SelectContent align="start" alignItemWithTrigger={false} className="min-w-[var(--radix-select-trigger-width)] p-1 bg-popover border border-border/80 rounded-xl shadow-2xl max-h-72">
                                {typedSchemaData.map(tb => (
                                  <SelectItem key={tb.name} value={tb.name} className="text-xs font-medium py-2 pl-3 cursor-pointer">{tb.name}</SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </div>

                          <div className="space-y-1.5">
                            <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                              {t('databaseStudio.structure.designer.targetColumn')}
                            </Label>
                            <Select value={editColFkTargetColumn} onValueChange={(val) => val && setEditColFkTargetColumn(val)}>
                              <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 text-xs font-semibold text-left">
                                <SelectValue placeholder={t('databaseStudio.structure.designer.selectColumn')} />
                              </SelectTrigger>
                              <SelectContent align="start" alignItemWithTrigger={false} className="min-w-[var(--radix-select-trigger-width)] p-1 bg-popover border border-border/80 rounded-xl shadow-2xl max-h-72">
                                {editColFkTargetTable && typedSchemaData.find(tb => tb.name === editColFkTargetTable)?.columns.map((c) => (
                                  <SelectItem key={c.name} value={c.name} className="text-xs font-medium py-2 pl-3 cursor-pointer">{c.name}</SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </div>
                        </div>

                        <div className="space-y-1.5">
                          <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                            {t('databaseStudio.structure.designer.onDeleteAction')}
                          </Label>
                          <Select value={editColFkOnDelete} onValueChange={(val) => val && setEditColFkOnDelete(val)}>
                            <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 text-xs font-semibold text-left">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent align="start" alignItemWithTrigger={false} className="min-w-[var(--radix-select-trigger-width)] p-1 bg-popover border border-border/80 rounded-xl shadow-2xl max-h-72">
                              <SelectItem value="CASCADE" className="text-xs font-medium py-2 pl-3 cursor-pointer">CASCADE</SelectItem>
                              <SelectItem value="SET NULL" className="text-xs font-medium py-2 pl-3 cursor-pointer">SET NULL</SelectItem>
                              <SelectItem value="RESTRICT" className="text-xs font-medium py-2 pl-3 cursor-pointer">RESTRICT</SelectItem>
                              <SelectItem value="NO ACTION" className="text-xs font-medium py-2 pl-3 cursor-pointer">NO ACTION</SelectItem>
                            </SelectContent>
                          </Select>
                        </div>
                      </div>
                    )}
                  </div>

                  <div className="space-y-1.5">
                    <Label htmlFor="mod_col_comment" className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                      {t('databaseStudio.structure.designer.commentLabel')}
                    </Label>
                    <Input
                      id="mod_col_comment"
                      value={editColComment}
                      onChange={(e) => setEditColComment(e.target.value)}
                      placeholder={t('databaseStudio.structure.designer.commentPlaceholder')}
                      className="h-10 rounded-xl bg-background/50 text-xs"
                    />
                  </div>
                </div>

                <div className="flex gap-2.5 pt-2 border-t border-border/40">
                  <Button type="submit" disabled={isActionLoading || !editColNewName.trim()} className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {t('databaseStudio.structure.modifyColumnDialog.submitBtn')}
                  </Button>
                  <Button type="button" onClick={() => resetModifyColumnForm()} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {t('common.cancel')}
                  </Button>
                </div>
              </form>
            ) : (
              <div className="space-y-4 pt-3">
                <div className="bg-muted/30 border border-border/60 rounded-xl p-3.5 font-mono text-xs text-foreground/80 whitespace-pre-wrap select-all max-h-[220px] overflow-y-auto leading-relaxed scrollbar-thin">
                  {colModifyPreviewSql}
                </div>

                <div className="flex gap-2.5 pt-2 border-t border-border/40">
                  <Button onClick={handleCommitModifyColumn} disabled={isActionLoading} className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {isActionLoading ? t('common.executing') : t('databaseStudio.structure.modifyColumnDialog.commitBtn')}
                  </Button>
                  <Button type="button" onClick={() => setShowColModifyPreview(false)} variant="outline" className="font-bold flex-1 rounded-xl cursor-pointer" style={{ cursor: 'pointer' }}>
                    {t('databaseStudio.structure.modifyColumnDialog.backBtn')}
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
