import React, { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { secretStoreAPI, projectsAPI } from '@/services/api'
import {
  Plus,
  FolderKey,
  Key,
  Trash2,
  Eye,
  EyeOff,
  Link2,
  Link2Off,
  Settings,
  Download,
  Upload,
  History,
  Loader2,
  Copy,
  Check,
  ChevronRight,
  Info
} from 'lucide-react'
import useTranslation from '@/lib/useTranslation'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import ConfirmationModal from '@/components/ConfirmationModal'

interface SecretStore {
  id: number
  user_id: number
  name: string
  description: string
  is_disabled: boolean
  created_at: string
  updated_at: string
}

interface SecretStoreItem {
  id: number
  key: string
  latest_snapshot_version: number
  created_at: string
  updated_at: string
}

interface SecretStoreItemValue {
  id: number
  secret_store_item_id: number
  version: number
  created_at: string
}

interface SecretStoreBinding {
  id: number
  secret_store_id: number
  project_id: number
  environment: string
  created_at: string
  project: {
    name: string
    subdomain: string
    uid: string
  }
}

interface ProjectOption {
  id: number
  uid: string
  name: string
  subdomain: string
}

export default function SecretStoreDashboard() {
  const { t } = useTranslation()
  
  // State variables
  const [stores, setStores] = useState<SecretStore[]>([])
  const [selectedStore, setSelectedStore] = useState<SecretStore | null>(null)
  const [items, setItems] = useState<SecretStoreItem[]>([])
  const [bindings, setBindings] = useState<SecretStoreBinding[]>([])
  const [projects, setProjects] = useState<ProjectOption[]>([])
  
  const [isLoadingStores, setIsLoadingStores] = useState(true)
  const [isLoadingDetails, setIsLoadingDetails] = useState(false)
  const [revealedValues, setRevealedValues] = useState<Record<number, string>>({})
  const [revealedHistoryValues, setRevealedHistoryValues] = useState<Record<number, string>>({})
  const [copiedKey, setCopiedKey] = useState<number | null>(null)

  const showError = (error: unknown, fallbackKey: string) => {
    const err = error as { response?: { data?: { code?: string; error?: string } } }
    const errorCode = err.response?.data?.code

    if (errorCode) {
      switch (errorCode) {
        case 'KEY_COLLISION':
          toast.error(t('secretstore.errors.collision'))
          return
        case 'NOT_FOUND':
          toast.error(t('secretstore.errors.notFound'))
          return
        case 'INVALID_ID':
          toast.error(t('secretstore.errors.invalidId'))
          return
        case 'INVALID_ITEM_ID':
        case 'ITEM_NOT_FOUND':
          toast.error(t('secretstore.errors.itemNotFound'))
          return
        case 'VALUE_NOT_FOUND':
          toast.error(t('secretstore.errors.valueNotFound'))
          return
        case 'DECRYPTION_FAILED':
          toast.error(t('secretstore.errors.decryptionFailed'))
          return
        case 'PROJECT_NOT_FOUND':
          toast.error(t('secretstore.errors.projectNotFound'))
          return
      }
    }

    toast.error(t(fallbackKey))
  }
  
  // Modals state
  const [isStoreModalOpen, setIsStoreModalOpen] = useState(false)
  const [storeForm, setStoreForm] = useState({ name: '', description: '' })
  const [editingStoreId, setEditingStoreId] = useState<number | null>(null)
  
  const [isVarModalOpen, setIsVarModalOpen] = useState(false)
  const [varForm, setVarForm] = useState({ key: '', value: '' })
  const [editingVarId, setEditingVarId] = useState<number | null>(null)
  
  const [isBindingModalOpen, setIsBindingModalOpen] = useState(false)
  const [bindingForm, setBindingForm] = useState({ projectUid: '', environment: 'all' })
  
  const [isHistoryModalOpen, setIsHistoryModalOpen] = useState(false)
  const [selectedItemHistory, setSelectedItemHistory] = useState<SecretStoreItemValue[]>([])
  const [activeHistoryItem, setActiveHistoryItem] = useState<SecretStoreItem | null>(null)
  
  const [isImportModalOpen, setIsImportModalOpen] = useState(false)
  const [importText, setImportText] = useState('')
  
  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '' as React.ReactNode,
    type: 'warning' as 'danger' | 'warning' | 'info',
    onConfirm: () => {},
    confirmText: t('common.confirm')
  })

  // Load stores list
  const fetchStores = useCallback(async (selectId?: number) => {
    setIsLoadingStores(true)
    try {
      const response = await secretStoreAPI.list()
      const data = response.data.data || []
      setStores(data)
      
      if (selectId) {
        const found = data.find((s: SecretStore) => s.id === selectId)
        if (found) setSelectedStore(found)
      } else if (data.length > 0) {
        setSelectedStore(prev => prev || data[0])
      }
    } catch (error) {
      toast.error(t('common.loadError'))
    } finally {
      setIsLoadingStores(false)
    }
  }, [t])

  // Load select projects list for binding options
  const fetchProjects = useCallback(async () => {
    try {
      const response = await projectsAPI.listOwn()
      setProjects(response.data.data || [])
    } catch (error) {
      console.error('Failed to load project bindings list', error)
    }
  }, [])

  useEffect(() => {
    fetchStores()
    fetchProjects()
  }, [fetchStores, fetchProjects])

  // Load active details when store changes
  const fetchStoreDetails = useCallback(async () => {
    if (!selectedStore) return
    setIsLoadingDetails(true)
    setRevealedValues({})
    try {
      const response = await secretStoreAPI.get(selectedStore.id)
      const data = response.data.data
      setItems(data.items || [])
      setBindings(data.bindings || [])
    } catch (error) {
      toast.error(t('common.loadError'))
    } finally {
      setIsLoadingDetails(false)
    }
  }, [selectedStore, t])

  useEffect(() => {
    fetchStoreDetails()
  }, [fetchStoreDetails])

  const copyToClipboard = (text: string, itemId: number) => {
    navigator.clipboard.writeText(text)
    setCopiedKey(itemId)
    toast.success(t('common.copySuccess'))
    setTimeout(() => setCopiedKey(null), 2000)
  }

  // Secret Store CRUD
  const handleSaveStore = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!storeForm.name.trim()) return
    
    try {
      if (editingStoreId) {
        await secretStoreAPI.update(editingStoreId, storeForm)
        toast.success(t('secretstore.updateSuccess'))
        await fetchStores(editingStoreId)
      } else {
        const res = await secretStoreAPI.create(storeForm)
        toast.success(t('secretstore.createSuccess'))
        await fetchStores(res.data.data.id)
      }
      setIsStoreModalOpen(false)
      setStoreForm({ name: '', description: '' })
      setEditingStoreId(null)
    } catch (error) {
      showError(error, 'common.error')
    }
  }

  const handleDeleteStore = (store: SecretStore) => {
    setConfirmModal({
      isOpen: true,
      title: t('secretstore.deleteConfirm'),
      message: t('secretstore.deleteConfirm'),
      type: 'danger',
      confirmText: t('common.delete'),
      onConfirm: async () => {
        try {
          await secretStoreAPI.delete(store.id)
          toast.success(t('secretstore.deleteSuccess'))
          setSelectedStore(null)
          fetchStores()
        } catch (error) {
          showError(error, 'common.error')
        }
      }
    })
  }

  // Variable CRUD
  const handleSaveVariable = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedStore || !varForm.key.trim() || !varForm.value.trim()) return

    try {
      if (editingVarId) {
        await secretStoreAPI.updateItem(selectedStore.id, editingVarId, { value: varForm.value })
        toast.success(t('secretstore.saveSuccess'))
      } else {
        await secretStoreAPI.createItem(selectedStore.id, varForm)
        toast.success(t('secretstore.saveSuccess'))
      }
      setIsVarModalOpen(false)
      setVarForm({ key: '', value: '' })
      setEditingVarId(null)
      fetchStoreDetails()
    } catch (error) {
      showError(error, 'common.error')
    }
  }

  const handleRevealValue = async (item: SecretStoreItem) => {
    if (revealedValues[item.id]) {
      setRevealedValues(prev => {
        const copy = { ...prev }
        delete copy[item.id]
        return copy
      })
      return
    }

    try {
      if (!selectedStore) return
      const res = await secretStoreAPI.revealItemValue(selectedStore.id, item.id)
      setRevealedValues(prev => ({ ...prev, [item.id]: res.data.data.value }))
    } catch (error) {
      showError(error, 'common.error')
    }
  }

  const handleDeleteVariable = (item: SecretStoreItem) => {
    if (!selectedStore) return
    setConfirmModal({
      isOpen: true,
      title: t('secretstore.confirmDeleteVar'),
      message: t('secretstore.confirmDeleteVar'),
      type: 'danger',
      confirmText: t('common.delete'),
      onConfirm: async () => {
        try {
          await secretStoreAPI.deleteItem(selectedStore.id, item.id)
          toast.success(t('common.deleteSuccess'))
          fetchStoreDetails()
        } catch (error) {
          showError(error, 'common.error')
        }
      }
    })
  }

  // Variable version history
  const handleViewHistory = async (item: SecretStoreItem) => {
    if (!selectedStore) return
    setRevealedHistoryValues({})
    try {
      const res = await secretStoreAPI.getItemHistory(selectedStore.id, item.id)
      setSelectedItemHistory(res.data.data || [])
      setActiveHistoryItem(item)
      setIsHistoryModalOpen(true)
    } catch (error) {
      showError(error, 'common.loadError')
    }
  }

  const handleRevealHistoryValue = async (hVal: SecretStoreItemValue) => {
    if (revealedHistoryValues[hVal.id]) {
      setRevealedHistoryValues(prev => {
        const copy = { ...prev }
        delete copy[hVal.id]
        return copy
      })
      return
    }

    try {
      if (!selectedStore || !activeHistoryItem) return
      const res = await secretStoreAPI.revealItemValue(selectedStore.id, activeHistoryItem.id, hVal.version)
      setRevealedHistoryValues(prev => ({ ...prev, [hVal.id]: res.data.data.value }))
    } catch (error) {
      showError(error, 'common.error')
    }
  }

  // Project Bindings CRUD
  const handleAddBinding = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedStore || !bindingForm.projectUid) return

    try {
      await secretStoreAPI.addBinding(selectedStore.id, {
        project_uid: bindingForm.projectUid,
        environment: bindingForm.environment
      })
      toast.success(t('common.success'))
      setIsBindingModalOpen(false)
      setBindingForm({ projectUid: '', environment: 'all' })
      fetchStoreDetails()
    } catch (error) {
      showError(error, 'common.error')
    }
  }

  const handleRemoveBinding = (binding: SecretStoreBinding) => {
    if (!selectedStore) return
    setConfirmModal({
      isOpen: true,
      title: t('common.confirm'),
      message: t('common.confirm'),
      type: 'danger',
      confirmText: t('common.delete'),
      onConfirm: async () => {
        try {
          await secretStoreAPI.removeBinding(selectedStore.id, binding.id)
          toast.success(t('common.deleteSuccess'))
          fetchStoreDetails()
        } catch (error) {
          showError(error, 'common.error')
        }
      }
    })
  }

  // Export Store
  const handleExportStore = async () => {
    if (!selectedStore) return
    try {
      const res = await secretStoreAPI.exportStore(selectedStore.id)
      const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(res.data.secrets, null, 2))
      const downloadAnchor = document.createElement('a')
      downloadAnchor.setAttribute("href", dataStr)
      downloadAnchor.setAttribute("download", `secretstore_${selectedStore.name.toLowerCase().replace(/\s+/g, '_')}_backup.json`)
      document.body.appendChild(downloadAnchor)
      downloadAnchor.click()
      downloadAnchor.remove()
      toast.success(t('secretstore.exportSuccess'))
    } catch (error) {
      toast.error(t('common.error'))
    }
  }

  // Import Store
  const handleImportStore = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!selectedStore || !importText.trim()) return

    try {
      const parsed = JSON.parse(importText)
      await secretStoreAPI.importStore(selectedStore.id, { secrets: parsed })
      toast.success(t('secretstore.importSuccess'))
      setIsImportModalOpen(false)
      setImportText('')
      fetchStoreDetails()
    } catch (error) {
      toast.error(t('secretstore.errors.invalidJson'))
    }
  }

  return (
    <div className="space-y-6 animate-in fade-in duration-500 pb-10">
      <ConfirmationModal
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
      />

      {/* Title Header */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4 pb-4 border-b border-border/60">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-1">{t('secretstore.title')}</h1>
          <p className="text-sm text-muted-foreground">{t('secretstore.desc')}</p>
        </div>
        <Button 
          size="sm"
          onClick={() => {
            setEditingStoreId(null)
            setStoreForm({ name: '', description: '' })
            setIsStoreModalOpen(true)
          }}
          className="w-full md:w-auto font-semibold uppercase tracking-wider text-[10px] h-9"
        >
          <Plus className="w-3.5 h-3.5 mr-2" />
          {t('secretstore.newStore')}
        </Button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
        {/* Left column: Stores List */}
        <div className="lg:col-span-1 space-y-3">
          {isLoadingStores ? (
            <div className="flex justify-center py-12">
              <Loader2 className="w-6 h-6 animate-spin text-primary/50" />
            </div>
          ) : stores.length === 0 ? (
            <Card className="border-dashed border-border/60">
              <CardContent className="pt-6 text-center text-xs text-muted-foreground">
                {t('secretstore.noStores')}
              </CardContent>
            </Card>
          ) : (
            stores.map(store => (
              <button
                key={store.id}
                onClick={() => setSelectedStore(store)}
                className={`w-full text-left px-4 py-3 rounded-lg border flex items-center justify-between transition-all group ${
                  selectedStore?.id === store.id
                    ? 'bg-secondary/40 border-primary/20 text-foreground'
                    : 'border-border/50 hover:bg-muted/30 text-muted-foreground hover:text-foreground'
                }`}
              >
                <div className="flex items-center gap-3 truncate">
                  <FolderKey className={`w-4 h-4 shrink-0 ${selectedStore?.id === store.id ? 'text-primary' : 'opacity-60'}`} />
                  <div className="truncate">
                    <p className="font-bold text-xs truncate">{store.name}</p>
                    <p className="text-[10px] text-muted-foreground truncate">{store.description || t('secretstore.noDescription')}</p>
                  </div>
                </div>
                <ChevronRight className={`w-3.5 h-3.5 shrink-0 transition-transform duration-200 group-hover:translate-x-0.5 ${
                  selectedStore?.id === store.id ? 'opacity-80' : 'opacity-0 group-hover:opacity-65'
                }`} />
              </button>
            ))
          )}
        </div>

        {/* Right column: Selected Store details */}
        <div className="lg:col-span-3">
          {selectedStore ? (
            <Card className="border-border/50 shadow-sm overflow-hidden h-full">
              <CardHeader className="pb-4 border-b border-border bg-card flex flex-row items-center justify-between">
                <div>
                  <CardTitle className="text-base flex items-center gap-2">
                    <FolderKey className="w-4 h-4 text-primary" />
                    {selectedStore.name}
                  </CardTitle>
                  <CardDescription className="text-xs">{selectedStore.description || t('secretstore.noDescriptionProvided')}</CardDescription>
                </div>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleExportStore}
                    className="h-8 text-[10px] font-bold uppercase tracking-wider px-3"
                    title={t('secretstore.export')}
                  >
                    <Download className="w-3.5 h-3.5 mr-1.5" />
                    {t('secretstore.export')}
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setIsImportModalOpen(true)}
                    className="h-8 text-[10px] font-bold uppercase tracking-wider px-3"
                    title={t('secretstore.import')}
                  >
                    <Upload className="w-3.5 h-3.5 mr-1.5" />
                    {t('secretstore.import')}
                  </Button>
                </div>
              </CardHeader>

              <Tabs defaultValue="variables" className="w-full">
                <div className="px-6 py-4 border-b border-border/50 bg-muted/5">
                  <TabsList className="flex border border-border/40 p-1 bg-muted/20 rounded-xl w-fit h-auto gap-1 bg-muted/20">
                    <TabsTrigger 
                      value="variables" 
                      className="px-5 py-2 rounded-lg text-xs font-bold uppercase tracking-wider transition-all duration-200 cursor-pointer h-auto border border-transparent data-active:!bg-background data-active:!text-primary data-active:!shadow-sm data-active:!border-border/40 !text-muted-foreground hover:!text-foreground dark:!text-muted-foreground dark:hover:!text-foreground dark:data-active:!text-primary dark:data-active:!bg-background"
                    >
                      {t('secretstore.variables')}
                    </TabsTrigger>
                    <TabsTrigger 
                      value="bindings" 
                      className="px-5 py-2 rounded-lg text-xs font-bold uppercase tracking-wider transition-all duration-200 cursor-pointer h-auto border border-transparent data-active:!bg-background data-active:!text-primary data-active:!shadow-sm data-active:!border-border/40 !text-muted-foreground hover:!text-foreground dark:!text-muted-foreground dark:hover:!text-foreground dark:data-active:!text-primary dark:data-active:!bg-background"
                    >
                      {t('secretstore.bindings')}
                    </TabsTrigger>
                    <TabsTrigger 
                      value="settings" 
                      className="px-5 py-2 rounded-lg text-xs font-bold uppercase tracking-wider transition-all duration-200 cursor-pointer h-auto border border-transparent data-active:!bg-background data-active:!text-primary data-active:!shadow-sm data-active:!border-border/40 !text-muted-foreground hover:!text-foreground dark:!text-muted-foreground dark:hover:!text-foreground dark:data-active:!text-primary dark:data-active:!bg-background"
                    >
                      {t('common.settings')}
                    </TabsTrigger>
                  </TabsList>
                </div>

                <CardContent className="p-6">
                  {/* Variables Tab */}
                  <TabsContent value="variables" className="space-y-4 m-0 outline-none">
                    <div className="flex justify-between items-center pb-2">
                      <h3 className="font-bold text-xs uppercase tracking-wider text-muted-foreground">{t('secretstore.storeVariables')}</h3>
                      <Button
                        size="sm"
                        onClick={() => {
                          setEditingVarId(null)
                          setVarForm({ key: '', value: '' })
                          setIsVarModalOpen(true)
                        }}
                        className="h-8 text-[10px] font-bold uppercase tracking-wider px-4"
                      >
                        <Plus className="w-3.5 h-3.5 mr-1.5" />
                        {t('secretstore.addVariable')}
                      </Button>
                    </div>

                    {isLoadingDetails ? (
                      <div className="flex justify-center py-20">
                        <Loader2 className="w-8 h-8 animate-spin text-primary/50" />
                      </div>
                    ) : items.length === 0 ? (
                      <div className="text-center py-20 border border-dashed border-border/50 rounded-lg text-xs text-muted-foreground">
                        {t('secretstore.noVariablesStored')}
                      </div>
                    ) : (
                      <div className="border border-border/50 rounded-lg overflow-hidden">
                        <Table>
                          <TableHeader className="bg-muted/20">
                            <TableRow>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/3">{t('secretstore.key')}</TableHead>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/2">{t('secretstore.value')}</TableHead>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px] text-right">{t('common.actions')}</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {items.map(item => {
                              const isRevealed = !!revealedValues[item.id]
                              const rawVal = revealedValues[item.id] || ''
                              return (
                                <TableRow key={item.id} className="hover:bg-muted/10">
                                  <TableCell className="font-mono text-xs font-semibold text-foreground">
                                    {item.key}
                                  </TableCell>
                                  <TableCell className="font-mono text-xs text-muted-foreground select-none">
                                    {isRevealed ? (
                                      <span className="text-foreground font-medium select-text">{rawVal}</span>
                                    ) : (
                                      <span className="tracking-widest">••••••••••••••••</span>
                                    )}
                                  </TableCell>
                                  <TableCell className="text-right">
                                    <div className="flex justify-end gap-1.5">
                                      <Button
                                        variant="outline"
                                        size="icon"
                                        className="h-8 w-8 text-muted-foreground hover:text-foreground"
                                        onClick={() => handleRevealValue(item)}
                                        title={isRevealed ? t('secretstore.hide') : t('secretstore.reveal')}
                                      >
                                        {isRevealed ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                                      </Button>
                                      
                                      <Button
                                        variant="outline"
                                        size="icon"
                                        className={`h-8 w-8 text-muted-foreground hover:text-foreground ${
                                          !isRevealed ? 'opacity-40 cursor-not-allowed hover:text-muted-foreground' : ''
                                        }`}
                                        onClick={() => isRevealed && copyToClipboard(rawVal, item.id)}
                                        disabled={!isRevealed}
                                        title={isRevealed ? t('common.copy') : t('secretstore.revealToCopy')}
                                      >
                                        {copiedKey === item.id ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
                                      </Button>
 
                                      <Button
                                        variant="outline"
                                        size="icon"
                                        className="h-8 w-8 text-muted-foreground hover:text-foreground"
                                        onClick={() => {
                                          setEditingVarId(item.id)
                                          setVarForm({ key: item.key, value: '' })
                                          setIsVarModalOpen(true)
                                        }}
                                        title={t('secretstore.rotate')}
                                      >
                                        <History className="w-3.5 h-3.5" />
                                      </Button>
 
                                      <Button
                                        variant="outline"
                                        size="icon"
                                        className="h-8 w-8 text-muted-foreground hover:text-foreground"
                                        onClick={() => handleViewHistory(item)}
                                        title={t('secretstore.history')}
                                      >
                                        <Info className="w-3.5 h-3.5" />
                                      </Button>
 
                                      <Button
                                        variant="outline"
                                        size="icon"
                                        className="h-8 w-8 text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                                        onClick={() => handleDeleteVariable(item)}
                                        title={t('common.delete')}
                                      >
                                        <Trash2 className="w-3.5 h-3.5" />
                                      </Button>
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

                  {/* Bindings Tab */}
                  <TabsContent value="bindings" className="space-y-4 m-0 outline-none">
                    <div className="flex justify-between items-center pb-2">
                      <h3 className="font-bold text-xs uppercase tracking-wider text-muted-foreground">{t('secretstore.bindingsTitle')}</h3>
                      <Button
                        size="sm"
                        onClick={() => {
                          setBindingForm({ projectUid: '', environment: 'all' })
                          setIsBindingModalOpen(true)
                        }}
                        className="h-8 text-[10px] font-bold uppercase tracking-wider px-4"
                      >
                        <Link2 className="w-3.5 h-3.5 mr-1.5" />
                        {t('secretstore.addBinding')}
                      </Button>
                    </div>

                    {isLoadingDetails ? (
                      <div className="flex justify-center py-20">
                        <Loader2 className="w-8 h-8 animate-spin text-primary/50" />
                      </div>
                    ) : bindings.length === 0 ? (
                      <div className="text-center py-20 border border-dashed border-border/50 rounded-lg text-xs text-muted-foreground">
                        {t('secretstore.noProjectsLinked')}
                      </div>
                    ) : (
                      <div className="border border-border/50 rounded-lg overflow-hidden">
                        <Table>
                          <TableHeader className="bg-muted/20">
                            <TableRow>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px]">{t('common.projectName')}</TableHead>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px]">{t('common.url')}</TableHead>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px]">{t('secretstore.environment')}</TableHead>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px] text-right">{t('common.actions')}</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {bindings.map(binding => (
                              <TableRow key={binding.id} className="hover:bg-muted/10">
                                <TableCell className="font-semibold text-xs text-foreground">
                                  {binding.project?.name || `Project (ID: ${binding.project_id})`}
                                </TableCell>
                                <TableCell className="font-mono text-xs text-muted-foreground">
                                  {binding.project?.subdomain || '-'}
                                </TableCell>
                                <TableCell>
                                  <Badge variant="secondary" className="font-bold text-[9px] uppercase tracking-wider px-2 py-0.5">
                                    {binding.environment === 'all' || binding.environment === '' 
                                      ? t('secretstore.allEnvs') 
                                      : binding.environment}
                                  </Badge>
                                </TableCell>
                                <TableCell className="text-right">
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    className="h-7 px-3 text-[9px] font-bold uppercase tracking-wider hover:text-destructive hover:bg-destructive/10"
                                    onClick={() => handleRemoveBinding(binding)}
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

                  {/* Settings Tab */}
                  <TabsContent value="settings" className="space-y-6 m-0 outline-none">
                    <div className="space-y-4 max-w-xl">
                      <h3 className="font-bold text-xs uppercase tracking-wider text-muted-foreground">{t('secretstore.manageProperties')}</h3>
                      <div className="space-y-4 border border-border/60 rounded-lg p-5 bg-muted/10">
                        <div>
                          <p className="font-semibold text-xs mb-1">{t('secretstore.renameOrUpdate')}</p>
                          <p className="text-[11px] text-muted-foreground mb-4">{t('secretstore.modifyParams')}</p>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => {
                              setEditingStoreId(selectedStore.id)
                              setStoreForm({ name: selectedStore.name, description: selectedStore.description })
                              setIsStoreModalOpen(true)
                            }}
                            className="h-9 px-4 font-bold text-[10px] uppercase tracking-wider"
                          >
                            <Settings className="w-3.5 h-3.5 mr-1.5" />
                            {t('secretstore.editStore')}
                          </Button>
                        </div>

                        <div className="border-t border-border/50 pt-4 mt-4">
                          <p className="font-semibold text-xs mb-1 text-destructive">{t('secretstore.dangerZone')}</p>
                          <p className="text-[11px] text-muted-foreground mb-4">{t('secretstore.deleteWarning')}</p>
                          <Button
                            variant="destructive"
                            size="sm"
                            onClick={() => handleDeleteStore(selectedStore)}
                            className="h-9 px-4 font-bold text-[10px] uppercase tracking-wider"
                          >
                            <Trash2 className="w-3.5 h-3.5 mr-1.5" />
                            {t('common.delete')}
                          </Button>
                        </div>
                      </div>
                    </div>
                  </TabsContent>
                </CardContent>
              </Tabs>
            </Card>
          ) : (
            <Card className="flex flex-col items-center justify-center h-[500px] border-dashed border-border/60 text-center p-8 bg-muted/5">
              <FolderKey className="w-12 h-12 text-muted-foreground opacity-30 mb-4" />
              <h3 className="font-bold text-base mb-1">{t('secretstore.noStores')}</h3>
              <p className="text-xs text-muted-foreground max-w-xs mb-6">{t('secretstore.noStoresDesc')}</p>
              <Button 
                onClick={() => {
                  setEditingStoreId(null)
                  setStoreForm({ name: '', description: '' })
                  setIsStoreModalOpen(true)
                }}
                className="font-semibold uppercase tracking-wider text-[10px]"
              >
                <Plus className="w-3.5 h-3.5 mr-2" />
                {t('secretstore.newStore')}
              </Button>
            </Card>
          )}
        </div>
      </div>

      {/* dialog for Create/Edit Store */}
      <Dialog open={isStoreModalOpen} onOpenChange={setIsStoreModalOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-base flex items-center gap-2">
              <FolderKey className="w-4 h-4 text-primary" />
              {editingStoreId ? t('secretstore.editStore') : t('secretstore.newStore')}
            </DialogTitle>
            <DialogDescription className="text-xs">
              {t('secretstore.enterParams')}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSaveStore} className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="store-name" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('secretstore.storeName')}</Label>
              <Input
                id="store-name"
                value={storeForm.name}
                onChange={e => setStoreForm(prev => ({ ...prev, name: e.target.value }))}
                placeholder={t('secretstore.storeNamePlaceholder')}
                required
                className="h-9"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="store-desc" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('secretstore.description')}</Label>
              <Input
                id="store-desc"
                value={storeForm.description}
                onChange={e => setStoreForm(prev => ({ ...prev, description: e.target.value }))}
                placeholder={t('secretstore.descPlaceholder')}
                className="h-9"
              />
            </div>
            <DialogFooter className="pt-2">
              <Button type="button" variant="outline" size="sm" className="h-9 font-semibold uppercase tracking-wider text-[10px]" onClick={() => setIsStoreModalOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" className="h-9 px-6 font-semibold uppercase tracking-wider text-[10px]">
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* dialog for Add/Edit Variable (Key-Value) */}
      <Dialog open={isVarModalOpen} onOpenChange={setIsVarModalOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-base flex items-center gap-2">
              <Key className="w-4 h-4 text-primary" />
              {editingVarId ? t('secretstore.rotate') : t('secretstore.addVariable')}
            </DialogTitle>
            <DialogDescription className="text-xs">
              {editingVarId 
                ? t('secretstore.rotateNotice')
                : t('secretstore.encryptionNotice')}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSaveVariable} className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="var-key" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('secretstore.key')}</Label>
              <Input
                id="var-key"
                value={varForm.key}
                onChange={e => setVarForm(prev => ({ ...prev, key: e.target.value }))}
                placeholder={t('secretstore.keyPlaceholder')}
                required
                disabled={!!editingVarId} // Disable key editing during rotation
                className="h-9 font-mono"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="var-val" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('secretstore.value')}</Label>
              <Input
                id="var-val"
                type="password"
                value={varForm.value}
                onChange={e => setVarForm(prev => ({ ...prev, value: e.target.value }))}
                placeholder={t('secretstore.valuePlaceholder')}
                required
                className="h-9"
              />
            </div>
            <DialogFooter className="pt-2">
              <Button type="button" variant="outline" size="sm" className="h-9 font-semibold uppercase tracking-wider text-[10px]" onClick={() => setIsVarModalOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" className="h-9 px-6 font-semibold uppercase tracking-wider text-[10px]">
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* dialog for Add Project Binding */}
      <Dialog open={isBindingModalOpen} onOpenChange={setIsBindingModalOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-base flex items-center gap-2">
              <Link2 className="w-4 h-4 text-primary" />
              {t('secretstore.addBinding')}
            </DialogTitle>
            <DialogDescription className="text-xs">
              {t('secretstore.injectNotice')}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleAddBinding} className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="bind-project" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('secretstore.selectProject')}</Label>
              <Select
                value={bindingForm.projectUid}
                onValueChange={val => setBindingForm(prev => ({ ...prev, projectUid: val || '' }))}
              >
                <SelectTrigger id="bind-project" className="w-full h-9 px-3 text-xs bg-background/50 border-border hover:border-border/80">
                  <div className="flex items-center gap-2 text-left flex-1 min-w-0 pr-4">
                    {(() => {
                      const p = projects.find(proj => proj.uid === bindingForm.projectUid)
                      return p ? (
                        <span className="truncate font-semibold text-foreground/90">{p.name}</span>
                      ) : (
                        <span className="text-muted-foreground/60">{t('secretstore.chooseProject')}</span>
                      )
                    })()}
                  </div>
                </SelectTrigger>
                <SelectContent align="start" alignItemWithTrigger={false} className="bg-popover border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72 min-w-[var(--anchor-width)] w-[var(--anchor-width)]">
                  {projects.map(p => (
                    <SelectItem key={p.uid} value={p.uid} className="rounded-lg py-2 px-3 cursor-pointer">
                      <div className="flex flex-col text-left">
                        <span className="font-semibold text-foreground text-xs">{p.name}</span>
                        <span className="text-[10px] text-muted-foreground font-mono">{p.subdomain}</span>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="bind-env" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('secretstore.environment')}</Label>
              <Select
                value={bindingForm.environment}
                onValueChange={val => setBindingForm(prev => ({ ...prev, environment: val || 'all' }))}
              >
                <SelectTrigger id="bind-env" className="w-full h-9 px-3 text-xs bg-background/50 border-border hover:border-border/80">
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
              <Button type="button" variant="outline" size="sm" className="h-9 font-semibold uppercase tracking-wider text-[10px]" onClick={() => setIsBindingModalOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" className="h-9 px-6 font-semibold uppercase tracking-wider text-[10px]">
                {t('secretstore.linkProjectBtn')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* dialog for Import Secrets JSON */}
      <Dialog open={isImportModalOpen} onOpenChange={setIsImportModalOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-base flex items-center gap-2">
              <Upload className="w-4 h-4 text-primary" />
              {t('secretstore.import')}
            </DialogTitle>
            <DialogDescription className="text-xs">
              {t('secretstore.pasteJsonNotice')}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleImportStore} className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="import-data" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">{t('secretstore.jsonPayload')}</Label>
              <textarea
                id="import-data"
                value={importText}
                onChange={e => setImportText(e.target.value)}
                placeholder='{
  "API_KEY": "stripe_secret_key_here",
  "API_URL": "https://api.stripe.com"
}'
                required
                className="w-full h-40 p-3 bg-zinc-950 text-zinc-300 font-mono text-xs border border-border rounded-md outline-none resize-none"
              />
            </div>
            <DialogFooter className="pt-2">
              <Button type="button" variant="outline" size="sm" className="h-9 font-semibold uppercase tracking-wider text-[10px]" onClick={() => setIsImportModalOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" className="h-9 px-6 font-semibold uppercase tracking-wider text-[10px]">
                {t('secretstore.importJsonBtn')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* dialog for Version History */}
      <Dialog open={isHistoryModalOpen} onOpenChange={setIsHistoryModalOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-base flex items-center gap-2">
              <History className="w-4 h-4 text-primary" />
              {t('secretstore.history')}: {activeHistoryItem?.key}
            </DialogTitle>
            <DialogDescription className="text-xs">
              {t('secretstore.timelineNotice')}
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-60 overflow-y-auto custom-scrollbar border border-border/60 rounded-lg">
            <Table>
              <TableHeader className="bg-muted/20">
                <TableRow>
                  <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/5">{t('projectDetail.settings.version')}</TableHead>
                  <TableHead className="font-bold uppercase tracking-widest text-[9px] w-2/5">{t('secretstore.value')}</TableHead>
                  <TableHead className="font-bold uppercase tracking-widest text-[9px] w-2/5">{t('common.date')}</TableHead>
                  <TableHead className="font-bold uppercase tracking-widest text-[9px] text-right">{t('common.status')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                 {selectedItemHistory.map(hVal => {
                  const isHistoryRevealed = !!revealedHistoryValues[hVal.id]
                  const historyRawVal = revealedHistoryValues[hVal.id] || ''
                  return (
                    <TableRow key={hVal.id} className="hover:bg-muted/10">
                      <TableCell className="font-semibold text-xs">
                        v{hVal.version}
                      </TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground select-none">
                        <div className="flex items-center justify-between gap-2">
                          {isHistoryRevealed ? (
                            <span className="text-foreground font-medium select-text">{historyRawVal}</span>
                          ) : (
                            <span className="tracking-widest">••••••••••••••••</span>
                          )}
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            className="h-6 w-6 text-muted-foreground hover:text-foreground shrink-0 cursor-pointer"
                            onClick={() => handleRevealHistoryValue(hVal)}
                            title={isHistoryRevealed ? t('secretstore.hide') : t('secretstore.reveal')}
                          >
                            {isHistoryRevealed ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                          </Button>
                        </div>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {new Date(hVal.created_at).toLocaleString()}
                      </TableCell>
                      <TableCell className="text-right">
                        {hVal.version === activeHistoryItem?.latest_snapshot_version ? (
                          <Badge className="bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/10 border border-emerald-500/20 text-[9px] font-bold uppercase tracking-wider px-2 py-0.5">
                            {t('secretstore.active')}
                          </Badge>
                        ) : (
                          <span className="text-[10px] text-muted-foreground uppercase font-semibold">{t('secretstore.archived')}</span>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
          <DialogFooter className="pt-2">
            <Button type="button" size="sm" className="h-9 px-6 font-semibold uppercase tracking-wider text-[10px]" onClick={() => setIsHistoryModalOpen(false)}>
              {t('common.btnClose')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
