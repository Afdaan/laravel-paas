import React, { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { secretStoreAPI, projectsAPI } from '@/services/api'
import {
  Plus,
  Vault,
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
  const [copiedKey, setCopiedKey] = useState<number | null>(null)
  
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
      } else if (data.length > 0 && !selectedStore) {
        setSelectedStore(data[0])
      }
    } catch (error) {
      toast.error(t('common.loadError'))
    } finally {
      setIsLoadingStores(false)
    }
  }, [selectedStore, t])

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
  }, []) // Run once on load

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
  }, [selectedStore])

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
      toast.error(t('common.error'))
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
          toast.error(t('common.error'))
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
      toast.error(t('common.error'))
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
      toast.error(t('common.error'))
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
          toast.error(t('common.error'))
        }
      }
    })
  }

  // Variable version history
  const handleViewHistory = async (item: SecretStoreItem) => {
    if (!selectedStore) return
    try {
      const res = await secretStoreAPI.getItemHistory(selectedStore.id, item.id)
      setSelectedItemHistory(res.data.data || [])
      setActiveHistoryItem(item)
      setIsHistoryModalOpen(true)
    } catch (error) {
      toast.error(t('common.loadError'))
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
      toast.error(t('common.error'))
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
          toast.error(t('common.error'))
        }
      }
    })
  }

  // Export Store
  const handleExportStore = async () => {
    if (!selectedStore) return
    try {
      const res = await secretStoreAPI.exportStore(selectedStore.id)
      const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(res.data.data, null, 2))
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
      toast.error(t('common.error') + ': Invalid JSON format')
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
                  <Vault className={`w-4 h-4 shrink-0 ${selectedStore?.id === store.id ? 'text-primary' : 'opacity-60'}`} />
                  <div className="truncate">
                    <p className="font-bold text-xs truncate">{store.name}</p>
                    <p className="text-[10px] text-muted-foreground truncate">{store.description || 'No description'}</p>
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
                    <Vault className="w-4 h-4 text-primary" />
                    {selectedStore.name}
                  </CardTitle>
                  <CardDescription className="text-xs">{selectedStore.description || 'No description provided'}</CardDescription>
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
                <div className="px-6 border-b border-border bg-muted/10">
                  <TabsList variant="line" className="bg-transparent h-12 p-0 gap-6">
                    <TabsTrigger 
                      value="variables" 
                      className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent h-full px-0 font-bold uppercase tracking-widest text-[9px] text-muted-foreground data-[state=active]:text-foreground"
                    >
                      {t('secretstore.variables')}
                    </TabsTrigger>
                    <TabsTrigger 
                      value="bindings" 
                      className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent h-full px-0 font-bold uppercase tracking-widest text-[9px] text-muted-foreground data-[state=active]:text-foreground"
                    >
                      {t('secretstore.bindings')}
                    </TabsTrigger>
                    <TabsTrigger 
                      value="settings" 
                      className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent h-full px-0 font-bold uppercase tracking-widest text-[9px] text-muted-foreground data-[state=active]:text-foreground"
                    >
                      {t('common.settings')}
                    </TabsTrigger>
                  </TabsList>
                </div>

                <CardContent className="p-6">
                  {/* Variables Tab */}
                  <TabsContent value="variables" className="space-y-4 m-0 outline-none">
                    <div className="flex justify-between items-center pb-2">
                      <h3 className="font-bold text-xs uppercase tracking-wider text-muted-foreground">Store Variables</h3>
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
                        No variables stored. Click "Add Variable" to store your first key-value pair.
                      </div>
                    ) : (
                      <div className="border border-border/50 rounded-lg overflow-hidden">
                        <Table>
                          <TableHeader className="bg-muted/20">
                            <TableRow>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/3">Key</TableHead>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/2">Value</TableHead>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px] text-right">Actions</TableHead>
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
                                      
                                      {isRevealed && (
                                        <Button
                                          variant="outline"
                                          size="icon"
                                          className="h-8 w-8 text-muted-foreground hover:text-foreground"
                                          onClick={() => copyToClipboard(rawVal, item.id)}
                                          title={t('common.copy')}
                                        >
                                          {copiedKey === item.id ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
                                        </Button>
                                      )}
 
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
                        No projects linked. Link this SecretStore to a project to inject these secrets at container runtime.
                      </div>
                    ) : (
                      <div className="border border-border/50 rounded-lg overflow-hidden">
                        <Table>
                          <TableHeader className="bg-muted/20">
                            <TableRow>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px]">Project</TableHead>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px]">Subdomain</TableHead>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px]">Environment</TableHead>
                              <TableHead className="font-bold uppercase tracking-widest text-[9px] text-right">Actions</TableHead>
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
                                    Unlink
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
                      <h3 className="font-bold text-xs uppercase tracking-wider text-muted-foreground">Manage Store Properties</h3>
                      <div className="space-y-4 border border-border/60 rounded-lg p-5 bg-muted/10">
                        <div>
                          <p className="font-semibold text-xs mb-1">Rename or Update Store</p>
                          <p className="text-[11px] text-muted-foreground mb-4">Modify store identity parameters.</p>
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
                          <p className="font-semibold text-xs mb-1 text-destructive">Danger Zone</p>
                          <p className="text-[11px] text-muted-foreground mb-4">Permanently delete this secret container. All linked projects will immediately lose access to these values upon next rebuild.</p>
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
              <Vault className="w-12 h-12 text-muted-foreground opacity-30 mb-4" />
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
              <Vault className="w-4 h-4 text-primary" />
              {editingStoreId ? t('secretstore.editStore') : t('secretstore.newStore')}
            </DialogTitle>
            <DialogDescription className="text-xs">
              Enter name and description parameters.
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
                ? 'Rotating variables will preserve previous snapshots in database records.'
                : 'Variables are encrypted with AES-256-GCM prior to database insertion.'}
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
              Inject this SecretStore into the project environment dynamically.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleAddBinding} className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="bind-project" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">Select Project</Label>
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
                        <span className="text-muted-foreground/60">Choose a project...</span>
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
                Link Project
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
              Paste JSON object format mapping variables. Overwrites matching existing keys.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleImportStore} className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="import-data" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">JSON payload</Label>
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
                Import JSON
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
              Timeline of credentials versions and historical update timestamps.
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-60 overflow-y-auto custom-scrollbar border border-border/60 rounded-lg">
            <Table>
              <TableHeader className="bg-muted/20">
                <TableRow>
                  <TableHead className="font-bold uppercase tracking-widest text-[9px]">Version</TableHead>
                  <TableHead className="font-bold uppercase tracking-widest text-[9px]">Timestamp</TableHead>
                  <TableHead className="font-bold uppercase tracking-widest text-[9px] text-right">Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                 {selectedItemHistory.map(hVal => (
                  <TableRow key={hVal.id}>
                    <TableCell className="font-semibold text-xs">
                      v{hVal.version}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {new Date(hVal.created_at).toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right">
                      {hVal.version === activeHistoryItem?.latest_snapshot_version ? (
                        <Badge className="bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/10 border border-emerald-500/20 text-[9px] font-bold uppercase tracking-wider px-2 py-0.5">
                          Active
                        </Badge>
                      ) : (
                        <span className="text-[10px] text-muted-foreground uppercase font-semibold">Archived</span>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <DialogFooter className="pt-2">
            <Button type="button" size="sm" className="h-9 px-6 font-semibold uppercase tracking-wider text-[10px]" onClick={() => setIsHistoryModalOpen(false)}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
