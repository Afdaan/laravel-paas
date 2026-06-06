import { useState, useEffect, useCallback, useMemo } from 'react'
import { Card, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import {
  Eye,
  EyeOff,
  Save,
  Loader2,
  ShieldAlert,
  AlertTriangle,
  Link2,
  Link2Off,
  Grid,
  FileText,
  Copy,
  Check,
  Pencil,
  Trash2,
  Plus,
  Lock
} from 'lucide-react'
import { cn } from '@/lib/utils'
import useTranslation from '@/lib/useTranslation'
import { projectsAPI, secretStoreAPI } from '@/services/api'
import { toast } from 'sonner'
import ConfirmationModal from '@/components/ConfirmationModal'

interface EnvironmentEditorProps {
  uid: string
  onSave?: () => void
  hasDatabaseInstance?: boolean
}

interface BoundStore {
  id: number
  name: string
  description: string
  environment: string
  bindingId: number
}

interface VariableGridItem {
  Key: string
  Value: string
  Source: string
}

export function EnvironmentEditor({ uid, onSave, hasDatabaseInstance = false }: EnvironmentEditorProps) {
  const { t } = useTranslation()
  
  // State variables
  const [activeSubTab, setActiveSubTab] = useState('grid')
  const [isLoading, setIsLoading] = useState(true)
  const [isSavingEnv, setIsSavingEnv] = useState(false)
  const [isEnvHidden, setIsEnvHidden] = useState(true)
  const [initialContent, setInitialContent] = useState('')
  const [currentDotenv, setCurrentDotenv] = useState('')
  const [boundStores, setBoundStores] = useState<BoundStore[]>([])
  const [allStores, setAllStores] = useState<any[]>([])
  
  const [revealedKeys, setRevealedKeys] = useState<Record<string, boolean>>({})
  const [copiedKey, setCopiedKey] = useState<string | null>(null)
  
  // Modals state
  const [isLinkModalOpen, setIsLinkModalOpen] = useState(false)
  const [linkForm, setLinkForm] = useState({ storeId: '', environment: 'all' })
  const [isEditModalOpen, setIsEditModalOpen] = useState(false)
  const [isAddModalOpen, setIsAddModalOpen] = useState(false)
  const [editForm, setEditForm] = useState({ key: '', value: '' })
  const [addForm, setAddForm] = useState({ key: '', value: '' })
  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '' as React.ReactNode,
    type: 'warning' as 'danger' | 'warning' | 'info',
    onConfirm: () => { },
    confirmText: t('common.confirm')
  })

  // Memoized grid items parsed from currentDotenv
  const gridItems = useMemo(() => {
    const lines = currentDotenv.split('\n')
    const items: VariableGridItem[] = []
    lines.forEach((line: string) => {
      const trimmed = line.trim()
      if (trimmed === '' || trimmed.startsWith('#')) return
      const parts = trimmed.split('=')
      if (parts.length >= 2) {
        const key = parts[0].trim()
        let val = parts.slice(1).join('=').trim()
        // Trim surrounding quotes
        if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
          val = val.substring(1, val.length - 1)
        }
        const isDBKey = key.startsWith('DB_') || key === 'DATABASE_URL'
        const isPlatformKey = key === 'APP_NAME' || key === 'APP_URL'
        
        let source = 'secret_store'
        if (isPlatformKey) {
          source = 'system'
        } else if (hasDatabaseInstance && isDBKey) {
          source = 'db_auto'
        }

        items.push({
          Key: key,
          Value: val,
          Source: source
        })
      }
    })

    // Sort items: locked (system & db_auto) first, then unlocked (secret_store). Alphabetically within groups.
    items.sort((a, b) => {
      const aLocked = a.Source === 'system' || a.Source === 'db_auto'
      const bLocked = b.Source === 'system' || b.Source === 'db_auto'
      if (aLocked && !bLocked) return -1
      if (!aLocked && bLocked) return 1
      
      // If both are locked, prioritize system first, then db_auto
      if (aLocked && bLocked) {
        if (a.Source === 'system' && b.Source !== 'system') return -1
        if (a.Source !== 'system' && b.Source === 'system') return 1
      }
      
      return a.Key.localeCompare(b.Key)
    })

    return items
  }, [currentDotenv, hasDatabaseInstance])

  // Memoized map of initial values to track dirty/unsaved state per variable
  const initialMap = useMemo(() => {
    const map: Record<string, string> = {}
    const lines = initialContent.split('\n')
    lines.forEach((line: string) => {
      const trimmed = line.trim()
      if (trimmed === '' || trimmed.startsWith('#')) return
      const parts = trimmed.split('=')
      if (parts.length >= 2) {
        const key = parts[0].trim()
        let val = parts.slice(1).join('=').trim()
        if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
          val = val.substring(1, val.length - 1)
        }
        map[key] = val
      }
    })
    return map
  }, [initialContent])

  const hasChanges = currentDotenv !== initialContent

  // Load environment dotenv content and parse to grid items
  const loadEnv = useCallback(async () => {
    setIsLoading(true)
    try {
      // 1. Get raw dotenv content
      const response = await projectsAPI.getEnv(uid)
      const content = response.data.content || ''
      setInitialContent(content)
      setCurrentDotenv(content)

      // 2. Load SecretStores list to inspect which ones are bound to this project
      const storesRes = await secretStoreAPI.list()
      const stores = storesRes.data.data || []
      setAllStores(stores)

      const bounds: BoundStore[] = []
      for (const store of stores) {
        const detailRes = await secretStoreAPI.get(store.id)
        const storeDetails = detailRes.data.data
        const storeBindings = storeDetails.bindings || []
        
        // Find matching binding for this project uid
        const match = storeBindings.find((b: any) => b.project?.uid === uid)
        if (match) {
          bounds.push({
            id: store.id,
            name: store.name,
            description: store.description,
            environment: match.environment,
            bindingId: match.id
          })
        }
      }
      setBoundStores(bounds)
    } catch (error) {
      toast.error(t('common.loadError'))
    } finally {
      setIsLoading(false)
    }
  }, [uid, t])

  useEffect(() => {
    loadEnv()
  }, [uid, loadEnv])

  const copyToClipboard = (text: string, keyName: string) => {
    navigator.clipboard.writeText(text)
    setCopiedKey(keyName)
    toast.success(t('common.copySuccess'))
    setTimeout(() => setCopiedKey(null), 2000)
  }

  const toggleRevealKey = (key: string) => {
    setRevealedKeys(prev => ({ ...prev, [key]: !prev[key] }))
  }

  const handleEditVariable = (e: React.FormEvent) => {
    e.preventDefault()
    if (!editForm.key) return

    // Find the item in gridItems and update its value
    const updatedItems = gridItems.map(item => {
      if (item.Key === editForm.key) {
        return { ...item, Value: editForm.value }
      }
      return item
    })
    
    // Compile back to dotenv
    const newContent = updatedItems.map(item => `${item.Key}=${item.Value}`).join('\n')
    setCurrentDotenv(newContent)
    setIsEditModalOpen(false)
  }

  const handleAddVariable = (e: React.FormEvent) => {
    e.preventDefault()
    if (!addForm.key) return

    // Check if key already exists
    const exists = gridItems.some(item => item.Key.toUpperCase() === addForm.key.toUpperCase())
    if (exists) {
      toast.error(t('projectDetail.secrets.keyExists'))
      return
    }

    const newItem = {
      Key: addForm.key.trim().toUpperCase(),
      Value: addForm.value,
      Source: 'secret_store'
    }
    
    const updatedItems = [...gridItems, newItem]
    
    // Compile back to dotenv
    const newContent = updatedItems.map(item => `${item.Key}=${item.Value}`).join('\n')
    setCurrentDotenv(newContent)
    setIsAddModalOpen(false)
    setAddForm({ key: '', value: '' })
  }

  const handleDeleteVariable = (keyToDelete: string) => {
    const updatedItems = gridItems.filter(item => item.Key !== keyToDelete)
    const newContent = updatedItems.map(item => `${item.Key}=${item.Value}`).join('\n')
    setCurrentDotenv(newContent)
    toast.success(t('common.deleteSuccess'))
  }

  // Save dotenv content bulk
  const handleSaveEnv = async () => {
    setIsSavingEnv(true)
    try {
      await projectsAPI.updateEnv(uid, currentDotenv)
      toast.success(t('projectDetail.secrets.saveSuccess', { defaultValue: 'Environment variables updated and container restart initiated.' }))
      if (onSave) onSave()
      await loadEnv()
    } catch (error) {
      toast.error(t('common.error'))
    } finally {
      setIsSavingEnv(false)
    }
  }

  const triggerSave = () => {
    setConfirmModal({
      title: t('common.confirm'),
      message: t('projectDetail.secrets.restartWarning', { defaultValue: 'Saving environment variables will trigger a container restart.' }),
      type: 'warning',
      confirmText: t('common.save'),
      isOpen: true,
      onConfirm: handleSaveEnv
    })
  }

  const handleReset = () => {
    setCurrentDotenv(initialContent)
    toast.success(t('projectDetail.secrets.resetSuccess', { defaultValue: 'Changes reset successfully' }))
  }

  // Link / Unlink SecretStore
  const handleLinkStore = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!linkForm.storeId) return

    try {
      await secretStoreAPI.addBinding(linkForm.storeId, {
        project_uid: uid,
        environment: linkForm.environment
      })
      toast.success(t('common.success'))
      setIsLinkModalOpen(false)
      setLinkForm({ storeId: '', environment: 'all' })
      loadEnv()
      if (onSave) onSave()
    } catch (error) {
      toast.error(t('common.error'))
    }
  }

  const handleUnlinkStore = (store: BoundStore) => {
    setConfirmModal({
      isOpen: true,
      title: t('common.confirm'),
      message: t('projectDetail.secrets.unlinkConfirm'),
      type: 'danger',
      confirmText: t('common.delete'),
      onConfirm: async () => {
        try {
          await secretStoreAPI.removeBinding(store.id, store.bindingId)
          toast.success(t('common.deleteSuccess'))
          loadEnv()
          if (onSave) onSave()
        } catch (error) {
          toast.error(t('common.error'))
        }
      }
    })
  }

  const linkableStores = allStores.filter(
    store => !boundStores.some(bs => bs.id === store.id)
  )

  if (isLoading) {
    return (
      <Card className="flex items-center justify-center h-[600px] border-border/50 bg-card/50">
        <Loader2 className="w-8 h-8 animate-spin text-primary/50" />
      </Card>
    )
  }

  return (
    <>
      <ConfirmationModal
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
      />
      <Card className="relative flex flex-col h-[600px] overflow-hidden border-border/50 shadow-sm bg-card">
        <CardHeader className="pb-4 flex flex-row items-center justify-between border-b border-border bg-card">
          <div>
            <CardTitle className="text-lg flex items-center gap-2">
              <ShieldAlert className="w-5 h-5 text-primary" />
              {t('projectDetail.tabs.secrets')}
            </CardTitle>
            <CardDescription className="text-xs flex flex-col sm:flex-row sm:items-center gap-1 sm:gap-2">
              <span>{t('projectDetail.secrets.desc')}</span>
              <span className="hidden sm:inline text-muted-foreground/50">|</span>
              <span className="text-[10px] text-amber-600 dark:text-amber-500/80 font-bold uppercase tracking-wider flex items-center gap-1">
                <AlertTriangle size={12} className="shrink-0" />
                {t('projectDetail.secrets.redeployNote')}
              </span>
            </CardDescription>
          </div>
          <div className="flex items-center gap-3">
            {activeSubTab === 'bulk' && (
              <Button 
                variant="outline" 
                size="sm" 
                onClick={() => setIsEnvHidden(!isEnvHidden)} 
                className="h-9"
              >
                {isEnvHidden ? <Eye className="w-3.5 h-3.5 mr-2" /> : <EyeOff className="w-3.5 h-3.5 mr-2" />}
                <span className="text-[10px] font-bold uppercase tracking-wider">
                  {isEnvHidden ? t('projectDetail.actions.reveal') : t('projectDetail.actions.hide')}
                </span>
              </Button>
            )}
            {activeSubTab === 'stores' && (
              <Button
                size="sm"
                onClick={() => setIsLinkModalOpen(true)}
                className="h-9 font-semibold text-[10px] uppercase tracking-wider"
              >
                <Link2 className="w-3.5 h-3.5 mr-1.5" />
                {t('projectDetail.secrets.linkModalTitle')}
              </Button>
            )}
          </div>
        </CardHeader>

        <Tabs value={activeSubTab} onValueChange={setActiveSubTab} className="flex-1 flex flex-col overflow-hidden">
          <div className="px-6 py-4 border-b border-border/50 bg-muted/5">
            <TabsList className="flex border border-border/40 p-1 bg-muted/20 rounded-xl w-fit h-auto gap-1 bg-muted/20">
              <TabsTrigger 
                value="grid" 
                className="px-5 py-2.5 rounded-lg text-xs font-bold uppercase tracking-wider transition-all duration-200 cursor-pointer h-auto border border-transparent data-active:!bg-background data-active:!text-primary data-active:!shadow-sm data-active:!border-border/40 !text-muted-foreground hover:!text-foreground dark:!text-muted-foreground dark:hover:!text-foreground dark:data-active:!text-primary dark:data-active:!bg-background"
              >
                <Grid className="w-3.5 h-3.5 mr-1.5" />
                {t('projectDetail.secrets.gridTab')}
              </TabsTrigger>
              <TabsTrigger 
                value="bulk" 
                className="px-5 py-2.5 rounded-lg text-xs font-bold uppercase tracking-wider transition-all duration-200 cursor-pointer h-auto border border-transparent data-active:!bg-background data-active:!text-primary data-active:!shadow-sm data-active:!border-border/40 !text-muted-foreground hover:!text-foreground dark:!text-muted-foreground dark:hover:!text-foreground dark:data-active:!text-primary dark:data-active:!bg-background"
              >
                <FileText className="w-3.5 h-3.5 mr-1.5" />
                {t('projectDetail.secrets.bulkTab')}
              </TabsTrigger>
              <TabsTrigger 
                value="stores" 
                className="px-5 py-2.5 rounded-lg text-xs font-bold uppercase tracking-wider transition-all duration-200 cursor-pointer h-auto border border-transparent data-active:!bg-background data-active:!text-primary data-active:!shadow-sm data-active:!border-border/40 !text-muted-foreground hover:!text-foreground dark:!text-muted-foreground dark:hover:!text-foreground dark:data-active:!text-primary dark:data-active:!bg-background"
              >
                <Link2 className="w-3.5 h-3.5 mr-1.5" />
                {t('projectDetail.secrets.storesTab')}
              </TabsTrigger>
            </TabsList>
          </div>

          <div className="flex-1 overflow-y-auto custom-scrollbar p-6 bg-card">
            {/* Resolved Variables Grid */}
            <TabsContent value="grid" className="m-0 outline-none space-y-4">
              <div className="flex justify-between items-center mb-1">
                <span className="text-[10px] text-muted-foreground font-medium uppercase tracking-wider">
                  {t('projectDetail.secrets.gridDesc')}
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setAddForm({ key: '', value: '' })
                    setIsAddModalOpen(true)
                  }}
                  className="h-8 text-[10px] font-bold uppercase tracking-wider hover:bg-muted/10"
                >
                  <Plus className="w-3.5 h-3.5 mr-1" />
                  {t('projectDetail.secrets.addVariable')}
                </Button>
              </div>
              {gridItems.length === 0 ? (
                <div className="text-center py-20 border border-dashed border-border/50 rounded-lg text-xs text-muted-foreground">
                  {t('projectDetail.secrets.noVariables')}
                </div>
              ) : (
                <div className="border border-border/50 rounded-lg overflow-hidden">
                  <Table>
                    <TableHeader className="bg-muted/20">
                      <TableRow>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/3">{t('secretstore.key')}</TableHead>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/2">{t('projectDetail.secrets.resolvedValue')}</TableHead>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px]">{t('projectDetail.secrets.sourceLayer')}</TableHead>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px] text-right">{t('common.actions')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {gridItems.map(item => {
                        const isDirty = !initialMap.hasOwnProperty(item.Key) || initialMap[item.Key] !== item.Value
                        const isRevealed = !!revealedKeys[item.Key] || isDirty
                        return (
                          <TableRow 
                            key={item.Key} 
                            className={cn(
                              "hover:bg-muted/10 transition-colors",
                              isDirty && "bg-amber-500/[0.03] dark:bg-amber-500/[0.02] hover:bg-amber-500/[0.05]"
                            )}
                          >
                            <TableCell className="font-mono text-xs font-semibold text-foreground">
                              <div className="flex items-center gap-2">
                                {isDirty && (
                                  <span className="w-1.5 h-1.5 rounded-full bg-amber-500 animate-pulse shrink-0" title="Unsaved changes" />
                                )}
                                <span>{item.Key}</span>
                              </div>
                            </TableCell>
                            <TableCell className="font-mono text-xs text-muted-foreground select-none">
                              {isRevealed ? (
                                <span className={cn(
                                  "font-medium select-text",
                                  isDirty ? "text-amber-500 dark:text-amber-400 font-semibold" : "text-foreground"
                                )}>
                                  {item.Value}
                                </span>
                              ) : (
                                <span className="tracking-widest">••••••••••••••••</span>
                              )}
                            </TableCell>
                            <TableCell>
                              <Badge variant="outline" className={`text-[9px] font-bold uppercase tracking-wider px-2 py-0.5 ${
                                item.Source === 'system'
                                  ? 'text-indigo-500 border-indigo-500/20 bg-indigo-500/5 dark:text-indigo-400 dark:border-indigo-400/20 dark:bg-indigo-400/5'
                                  : item.Source === 'db_auto'
                                    ? 'text-primary border-primary/20 bg-primary/5'
                                    : 'text-zinc-400 border-border bg-muted/20'
                              }`}>
                                {item.Source === 'system'
                                  ? t('projectDetail.secrets.platformManaged')
                                  : item.Source === 'db_auto'
                                    ? t('projectDetail.secrets.dbAutoProvision')
                                    : t('projectDetail.secrets.secretStoreConfig')}
                              </Badge>
                            </TableCell>
                            <TableCell className="text-right">
                              <div className="flex justify-end gap-1.5">
                                <Button
                                  variant="outline"
                                  size="icon"
                                  disabled={isDirty}
                                  className={cn(
                                    "h-8 w-8 text-muted-foreground hover:text-foreground",
                                    isDirty && "opacity-50 cursor-not-allowed hover:text-muted-foreground"
                                  )}
                                  onClick={() => toggleRevealKey(item.Key)}
                                  title={isDirty ? t('projectDetail.secrets.cannotHideUnsaved') : (isRevealed ? t('secretstore.hide') : t('secretstore.reveal'))}
                                >
                                  {isRevealed ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                                </Button>
                                <Button
                                  variant="outline"
                                  size="icon"
                                  className="h-8 w-8 text-muted-foreground hover:text-foreground"
                                  onClick={() => copyToClipboard(item.Value, item.Key)}
                                  title={t('common.copy')}
                                >
                                  {copiedKey === item.Key ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
                                </Button>
                                 {item.Source === 'secret_store' ? (
                                  <>
                                    <Button
                                      variant="outline"
                                      size="icon"
                                      className="h-8 w-8 text-muted-foreground hover:text-foreground"
                                      onClick={() => {
                                        setEditForm({ key: item.Key, value: item.Value })
                                        setIsEditModalOpen(true)
                                      }}
                                      title={t('common.edit')}
                                    >
                                      <Pencil className="w-3.5 h-3.5" />
                                    </Button>
                                    <Button
                                      variant="outline"
                                      size="icon"
                                      className="h-8 w-8 text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                                      onClick={() => handleDeleteVariable(item.Key)}
                                      title={t('common.delete')}
                                    >
                                      <Trash2 className="w-3.5 h-3.5" />
                                    </Button>
                                  </>
                                ) : (
                                  <Button
                                    variant="outline"
                                    size="icon"
                                    disabled
                                    className="h-8 w-8 text-muted-foreground/30 border-border/40 bg-muted/5 cursor-not-allowed"
                                    title={t('projectDetail.secrets.lockTooltip')}
                                  >
                                    <Lock className="w-3.5 h-3.5" />
                                  </Button>
                                )}
                              </div>
                            </TableCell>
                          </TableRow>
                        )
                      })}
                    </TableBody>
                  </Table>
                </div>
              )}
            </TabsContent>

            {/* Bulk Editor Dotenv */}
            <TabsContent value="bulk" className="m-0 outline-none h-full flex flex-col relative bg-zinc-950 rounded-lg overflow-hidden border border-border/50">
              <textarea
                value={currentDotenv}
                onChange={e => setCurrentDotenv(e.target.value)}
                readOnly={isEnvHidden}
                spellCheck={false}
                autoComplete="off"
                autoCorrect="off"
                autoCapitalize="off"
                data-gramm="false"
                className={cn(
                  "w-full h-full p-6 font-mono text-[13px] leading-relaxed resize-none bg-transparent text-zinc-300 outline-none overflow-y-auto custom-scrollbar transition-all duration-300",
                  "selection:bg-primary/30 selection:text-white",
                  isEnvHidden ? "blur-md opacity-40 select-none pointer-events-none" : "opacity-100 blur-0"
                )}
                placeholder={t('projectDetail.secrets.placeholder')}
              />
              
              {isEnvHidden && (
                <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
                  <div className="px-6 py-3 bg-zinc-900/80 border border-white/10 backdrop-blur-md rounded-full shadow-2xl flex items-center gap-3">
                    <ShieldAlert className="w-4 h-4 text-primary" />
                    <span className="text-[10px] font-bold tracking-[0.2em] uppercase text-zinc-400">
                      {t('projectDetail.secrets.locked')}
                    </span>
                  </div>
                </div>
              )}
            </TabsContent>

            {/* Linked SecretStores Bindings */}
            <TabsContent value="stores" className="m-0 outline-none space-y-4">
              {boundStores.length === 0 ? (
                <div className="text-center py-20 border border-dashed border-border/50 rounded-lg text-xs text-muted-foreground">
                  {t('projectDetail.secrets.noBoundStores')}
                </div>
              ) : (
                <div className="border border-border/50 rounded-lg overflow-hidden">
                  <Table>
                    <TableHeader className="bg-muted/20">
                      <TableRow>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/3">{t('secretstore.storeName')}</TableHead>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/2">{t('secretstore.description')}</TableHead>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px]">{t('projectDetail.secrets.scopeEnv')}</TableHead>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px] text-right">{t('common.actions')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {boundStores.map(store => (
                        <TableRow key={store.id} className="hover:bg-muted/10">
                          <TableCell className="font-bold text-xs text-foreground">
                            {store.name}
                          </TableCell>
                          <TableCell className="text-xs text-muted-foreground">
                            {store.description || '-'}
                          </TableCell>
                          <TableCell>
                            <Badge variant="secondary" className="font-bold text-[9px] uppercase tracking-wider px-2 py-0.5">
                              {store.environment === 'production'
                                ? t('secretstore.prod')
                                : store.environment === 'staging'
                                  ? t('secretstore.staging')
                                  : store.environment === 'development'
                                    ? t('secretstore.dev')
                                    : t('secretstore.allEnvs')}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-right">
                            <Button
                              variant="outline"
                              size="sm"
                              className="h-7 px-3 text-[9px] font-bold uppercase tracking-wider hover:text-destructive hover:bg-destructive/10"
                              onClick={() => handleUnlinkStore(store)}
                            >
                              <Link2Off className="w-3.5 h-3.5 mr-1" />
                              {t('projectDetail.secrets.unlinkBtn')}
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </TabsContent>
          </div>

        </Tabs>

        {/* Floating Unsaved Changes Banner */}
        {hasChanges && (activeSubTab === 'grid' || activeSubTab === 'bulk') && (
          <div className="absolute bottom-8 left-1/2 -translate-x-1/2 z-50 bg-background/95 backdrop-blur-md border border-border/80 shadow-2xl rounded-full px-5 py-2.5 flex items-center gap-4 animate-in fade-in slide-in-from-bottom-3 duration-300">
            <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5 select-none">
              <span className="w-2 h-2 rounded-full bg-amber-500 animate-pulse" />
              {t('projectDetail.secrets.unsavedChanges')}
            </span>
            <div className="w-px h-4 bg-border/60" />
            <div className="flex gap-2">
              <Button
                variant="ghost"
                size="sm"
                onClick={handleReset}
                className="h-8 px-4 rounded-full text-[10px] font-bold uppercase tracking-wider hover:bg-muted/10 cursor-pointer"
              >
                {t('projectDetail.secrets.resetBtn')}
              </Button>
              <Button
                size="sm"
                onClick={triggerSave}
                disabled={isSavingEnv}
                className="h-8 px-5 rounded-full text-[10px] font-bold uppercase tracking-wider bg-primary hover:bg-primary/90 text-primary-foreground flex items-center gap-1.5 cursor-pointer shadow-md"
              >
                {isSavingEnv ? (
                  <Loader2 className="w-3 h-3 animate-spin" />
                ) : (
                  <Save className="w-3 h-3" />
                )}
                {t('common.save')}
              </Button>
            </div>
          </div>
        )}
      </Card>

      {/* dialog for Link SecretStore */}
      <Dialog open={isLinkModalOpen} onOpenChange={setIsLinkModalOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-base flex items-center gap-2">
              <Link2 className="w-4 h-4 text-primary" />
              {t('projectDetail.secrets.linkModalTitle')}
            </DialogTitle>
            <DialogDescription className="text-xs">
              {t('projectDetail.secrets.linkModalDesc')}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleLinkStore} className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="link-store" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('projectDetail.secrets.selectStore')}</Label>
              {linkableStores.length === 0 ? (
                <div className="text-xs text-muted-foreground py-2 border rounded-md px-3 bg-muted/10">
                  {t('projectDetail.secrets.noOtherStores')}
                </div>
              ) : (
                <Select
                  value={linkForm.storeId}
                  onValueChange={val => setLinkForm(prev => ({ ...prev, storeId: val || '' }))}
                >
                  <SelectTrigger id="link-store" className="w-full h-9 px-3 text-xs bg-background/50 border-border hover:border-border/80">
                    <div className="flex items-center gap-2 text-left flex-1 min-w-0 pr-4">
                      {(() => {
                        const s = linkableStores.find(store => String(store.id) === linkForm.storeId)
                        return s ? (
                          <span className="truncate font-semibold text-foreground/90">{s.name}</span>
                        ) : (
                          <span className="text-muted-foreground/60">{t('projectDetail.secrets.chooseStore')}</span>
                        )
                      })()}
                    </div>
                  </SelectTrigger>
                  <SelectContent align="start" alignItemWithTrigger={false} className="bg-popover border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72 min-w-[var(--anchor-width)] w-[var(--anchor-width)]">
                    {linkableStores.map(store => (
                      <SelectItem key={store.id} value={String(store.id)} className="rounded-lg py-2 px-3 cursor-pointer text-xs">
                        <div className="flex flex-col text-left">
                          <span className="font-semibold text-foreground text-xs">{store.name}</span>
                          {store.description && (
                            <span className="text-[10px] text-muted-foreground mt-0.5 truncate">{store.description}</span>
                          )}
                        </div>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="link-env" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('secretstore.environment')}</Label>
              <Select
                value={linkForm.environment}
                onValueChange={val => setLinkForm(prev => ({ ...prev, environment: val || 'all' }))}
              >
                <SelectTrigger id="link-env" className="w-full h-9 px-3 text-xs bg-background/50 border-border hover:border-border/80">
                  <div className="flex items-center gap-2 text-left flex-1 min-w-0 pr-4">
                    <SelectValue />
                  </div>
                </SelectTrigger>
                <SelectContent align="start" alignItemWithTrigger={false} className="bg-popover border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72 min-w-[var(--anchor-width)] w-[var(--anchor-width)]">
                  <SelectItem value="all" className="rounded-lg py-2 px-3 cursor-pointer text-xs">{t('secretstore.allEnvs')}</SelectItem>
                  <SelectItem value="production" className="rounded-lg py-2 px-3 cursor-pointer text-xs">{t('secretstore.prod')}</SelectItem>
                  <SelectItem value="staging" className="rounded-lg py-2 px-3 cursor-pointer text-xs">{t('secretstore.staging')}</SelectItem>
                  <SelectItem value="development" className="rounded-lg py-2 px-3 cursor-pointer text-xs">{t('secretstore.dev')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <DialogFooter className="pt-2">
              <Button type="button" variant="outline" size="sm" className="h-9 font-semibold uppercase tracking-wider text-[10px]" onClick={() => setIsLinkModalOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" disabled={!linkForm.storeId} className="h-9 px-6 font-semibold uppercase tracking-wider text-[10px]">
                {t('projectDetail.secrets.linkStoreBtn')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Edit Variable Modal */}
      <Dialog open={isEditModalOpen} onOpenChange={setIsEditModalOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-base flex items-center gap-2">
              <Pencil className="w-4 h-4 text-primary" />
              {t('projectDetail.secrets.editVariable')}
            </DialogTitle>
            <DialogDescription className="text-xs">
              {t('projectDetail.secrets.editVariableDesc')}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleEditVariable} className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="edit-key" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('projectDetail.secrets.variableKey')}</Label>
              <input
                id="edit-key"
                type="text"
                readOnly
                value={editForm.key}
                className="w-full h-9 px-3 text-xs bg-muted/20 border border-border rounded-lg text-muted-foreground outline-none cursor-not-allowed font-mono"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="edit-value" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('projectDetail.secrets.variableValue')}</Label>
              <input
                id="edit-value"
                type="text"
                value={editForm.value}
                onChange={e => setEditForm(prev => ({ ...prev, value: e.target.value }))}
                placeholder={t('secretstore.valuePlaceholder')}
                className="w-full h-9 px-3 text-xs bg-background/50 border border-border rounded-lg text-foreground outline-none hover:border-border/80 focus:border-primary/50 font-mono"
              />
            </div>
            <DialogFooter className="pt-2">
              <Button type="button" variant="outline" size="sm" className="h-9 font-semibold uppercase tracking-wider text-[10px]" onClick={() => setIsEditModalOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" className="h-9 px-6 font-semibold uppercase tracking-wider text-[10px]">
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Add Variable Modal */}
      <Dialog open={isAddModalOpen} onOpenChange={setIsAddModalOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-base flex items-center gap-2">
              <Plus className="w-4 h-4 text-primary" />
              {t('projectDetail.secrets.addVariable')}
            </DialogTitle>
            <DialogDescription className="text-xs">
              {t('projectDetail.secrets.addVariableDesc')}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleAddVariable} className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="add-key" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('projectDetail.secrets.variableKey')}</Label>
              <input
                id="add-key"
                type="text"
                required
                value={addForm.key}
                onChange={e => setAddForm(prev => ({ ...prev, key: e.target.value }))}
                placeholder={t('projectDetail.secrets.keyPlaceholder')}
                className="w-full h-9 px-3 text-xs bg-background/50 border border-border rounded-lg text-foreground outline-none hover:border-border/80 focus:border-primary/50 font-mono uppercase"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="add-value" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('projectDetail.secrets.variableValue')}</Label>
              <input
                id="add-value"
                type="text"
                required
                value={addForm.value}
                onChange={e => setAddForm(prev => ({ ...prev, value: e.target.value }))}
                placeholder={t('secretstore.valuePlaceholder')}
                className="w-full h-9 px-3 text-xs bg-background/50 border border-border rounded-lg text-foreground outline-none hover:border-border/80 focus:border-primary/50 font-mono"
              />
            </div>
            <DialogFooter className="pt-2">
              <Button type="button" variant="outline" size="sm" className="h-9 font-semibold uppercase tracking-wider text-[10px]" onClick={() => setIsAddModalOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" disabled={!addForm.key || !addForm.value} className="h-9 px-6 font-semibold uppercase tracking-wider text-[10px]">
                {t('projectDetail.secrets.addVariable')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
