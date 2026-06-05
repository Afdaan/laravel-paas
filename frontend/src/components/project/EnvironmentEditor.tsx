import { useState, useEffect, useRef, useCallback } from 'react'
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
  Check
} from 'lucide-react'
import { cn } from '@/lib/utils'
import useTranslation from '@/lib/useTranslation'
import { projectsAPI, secretStoreAPI } from '@/services/api'
import { toast } from 'sonner'
import ConfirmationModal from '@/components/ConfirmationModal'

interface EnvironmentEditorProps {
  uid: string
  onSave?: () => void
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

export function EnvironmentEditor({ uid, onSave }: EnvironmentEditorProps) {
  const { t } = useTranslation()
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  
  // State variables
  const [activeSubTab, setActiveSubTab] = useState('grid')
  const [isLoading, setIsLoading] = useState(true)
  const [isSavingEnv, setIsSavingEnv] = useState(false)
  const [isEnvHidden, setIsEnvHidden] = useState(true)
  const [initialContent, setInitialContent] = useState('')
  const [gridItems, setGridItems] = useState<VariableGridItem[]>([])
  const [boundStores, setBoundStores] = useState<BoundStore[]>([])
  const [allStores, setAllStores] = useState<any[]>([])
  
  const [revealedKeys, setRevealedKeys] = useState<Record<string, boolean>>({})
  const [copiedKey, setCopiedKey] = useState<string | null>(null)
  
  // Modals state
  const [isLinkModalOpen, setIsLinkModalOpen] = useState(false)
  const [linkForm, setLinkForm] = useState({ storeId: '', environment: 'all' })
  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '' as React.ReactNode,
    type: 'warning' as 'danger' | 'warning' | 'info',
    onConfirm: () => { },
    confirmText: t('common.confirm')
  })

  // Load environment dotenv content and parse to grid items
  const loadEnv = useCallback(async () => {
    setIsLoading(true)
    try {
      // 1. Get raw dotenv content
      const response = await projectsAPI.getEnv(uid)
      const content = response.data.content || ''
      setInitialContent(content)
      
      // Parse dotenv to key-value grid items
      const lines = content.split('\n')
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
          items.push({
            Key: key,
            Value: val,
            Source: key.startsWith('DB_') || key === 'DATABASE_URL' ? 'Database Auto-Provision' : 'Secret Store / Config'
          })
        }
      })
      setGridItems(items)

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

  // Save dotenv content bulk
  const handleSaveEnv = async () => {
    const content = textareaRef.current?.value || ''
    setIsSavingEnv(true)
    try {
      await projectsAPI.updateEnv(uid, content)
      toast.success(t('common.success'))
      if (onSave) onSave()
      loadEnv()
    } catch (error) {
      toast.error(t('common.error'))
    } finally {
      setIsSavingEnv(false)
    }
  }

  const triggerSave = () => {
    setConfirmModal({
      title: t('common.confirm'),
      message: t('projectDetail.settings.redeployWarning'),
      type: 'warning',
      confirmText: t('common.save'),
      isOpen: true,
      onConfirm: handleSaveEnv
    })
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
      message: 'Unlinking this store will remove its variables from your project upon next rebuild. Proceed?',
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
      <Card className="flex flex-col h-[600px] overflow-hidden border-border/50 shadow-sm bg-card">
        <CardHeader className="pb-4 flex flex-row items-center justify-between border-b border-border bg-card">
          <div>
            <CardTitle className="text-lg flex items-center gap-2">
              <ShieldAlert className="w-5 h-5 text-primary" />
              {t('projectDetail.tabs.secrets')}
            </CardTitle>
            <CardDescription className="text-xs">{t('projectDetail.secrets.desc')}</CardDescription>
          </div>
          <div className="flex items-center gap-3">
            {activeSubTab === 'bulk' && (
              <>
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
                <Button
                  size="sm"
                  onClick={triggerSave}
                  disabled={isSavingEnv || isEnvHidden}
                  className="h-9 px-6 bg-primary hover:bg-primary/90"
                >
                  {isSavingEnv ? <Loader2 className="w-3.5 h-3.5 mr-2 animate-spin" /> : <Save className="w-3.5 h-3.5 mr-2" />}
                  <span className="text-[10px] font-bold uppercase tracking-wider">{t('common.save')}</span>
                </Button>
              </>
            )}
            {activeSubTab === 'stores' && (
              <Button
                size="sm"
                onClick={() => setIsLinkModalOpen(true)}
                className="h-9 font-semibold text-[10px] uppercase tracking-wider"
              >
                <Link2 className="w-3.5 h-3.5 mr-1.5" />
                Link SecretStore
              </Button>
            )}
          </div>
        </CardHeader>

        <Tabs value={activeSubTab} onValueChange={setActiveSubTab} className="flex-1 flex flex-col overflow-hidden">
          <div className="px-6 border-b border-border bg-muted/10">
            <TabsList className="bg-transparent h-12 p-0 gap-6">
              <TabsTrigger 
                value="grid" 
                className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent h-full px-0 font-bold uppercase tracking-widest text-[9px] text-muted-foreground data-[state=active]:text-foreground"
              >
                <Grid className="w-3.5 h-3.5 mr-1.5" />
                Variables Grid
              </TabsTrigger>
              <TabsTrigger 
                value="bulk" 
                className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent h-full px-0 font-bold uppercase tracking-widest text-[9px] text-muted-foreground data-[state=active]:text-foreground"
              >
                <FileText className="w-3.5 h-3.5 mr-1.5" />
                Bulk Editor (Dotenv)
              </TabsTrigger>
              <TabsTrigger 
                value="stores" 
                className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent h-full px-0 font-bold uppercase tracking-widest text-[9px] text-muted-foreground data-[state=active]:text-foreground"
              >
                <Link2 className="w-3.5 h-3.5 mr-1.5" />
                Linked SecretStores
              </TabsTrigger>
            </TabsList>
          </div>

          <div className="flex-1 overflow-y-auto custom-scrollbar p-6 bg-card">
            {/* Resolved Variables Grid */}
            <TabsContent value="grid" className="m-0 outline-none space-y-4">
              {gridItems.length === 0 ? (
                <div className="text-center py-20 border border-dashed border-border/50 rounded-lg text-xs text-muted-foreground">
                  No environment variables defined. Add them using the Bulk Editor or link a SecretStore.
                </div>
              ) : (
                <div className="border border-border/50 rounded-lg overflow-hidden">
                  <Table>
                    <TableHeader className="bg-muted/20">
                      <TableRow>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/3">Key</TableHead>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/2">Resolved Value</TableHead>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px]">Source Layer</TableHead>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px] text-right">Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {gridItems.map(item => {
                        const isRevealed = !!revealedKeys[item.Key]
                        return (
                          <TableRow key={item.Key} className="hover:bg-muted/10">
                            <TableCell className="font-mono text-xs font-semibold text-foreground">
                              {item.Key}
                            </TableCell>
                            <TableCell className="font-mono text-xs text-muted-foreground select-none">
                              {isRevealed ? (
                                <span className="text-foreground font-medium select-text">{item.Value}</span>
                              ) : (
                                <span className="tracking-widest">••••••••••••••••</span>
                              )}
                            </TableCell>
                            <TableCell>
                              <Badge variant="outline" className={`text-[9px] font-bold uppercase tracking-wider px-2 py-0.5 ${
                                item.Source.includes('Auto-Provision')
                                  ? 'text-primary border-primary/20 bg-primary/5'
                                  : 'text-zinc-400 border-border bg-muted/20'
                              }`}>
                                {item.Source}
                              </Badge>
                            </TableCell>
                            <TableCell className="text-right">
                              <div className="flex justify-end gap-1.5">
                                <Button
                                  variant="outline"
                                  size="icon"
                                  className="h-8 w-8 text-muted-foreground hover:text-foreground"
                                  onClick={() => toggleRevealKey(item.Key)}
                                  title={isRevealed ? t('secretstore.hide') : t('secretstore.reveal')}
                                >
                                  {isRevealed ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                                </Button>
                                {isRevealed && (
                                  <Button
                                    variant="outline"
                                    size="icon"
                                    className="h-8 w-8 text-muted-foreground hover:text-foreground"
                                    onClick={() => copyToClipboard(item.Value, item.Key)}
                                    title={t('common.copy')}
                                  >
                                    {copiedKey === item.Key ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
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
                ref={textareaRef}
                defaultValue={initialContent}
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
                  No SecretStores bound to this project. Bind global user SecretStores to automatically inject credentials.
                </div>
              ) : (
                <div className="border border-border/50 rounded-lg overflow-hidden">
                  <Table>
                    <TableHeader className="bg-muted/20">
                      <TableRow>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/3">Store Name</TableHead>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/2">Description</TableHead>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px]">Scope Environment</TableHead>
                        <TableHead className="font-bold uppercase tracking-widest text-[9px] text-right">Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {boundStores.map(store => (
                        <TableRow key={store.id} className="hover:bg-muted/10">
                          <TableCell className="font-bold text-xs text-foreground">
                            {store.name}
                          </TableCell>
                          <TableCell className="text-xs text-muted-foreground">
                            {store.description || 'No description'}
                          </TableCell>
                          <TableCell>
                            <Badge variant="secondary" className="font-bold text-[9px] uppercase tracking-wider px-2 py-0.5">
                              {store.environment === 'all' || store.environment === ''
                                ? 'All Environments'
                                : store.environment}
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
          </div>

          <div className="p-3 bg-amber-500/5 text-amber-600 dark:text-amber-500/80 text-[9px] font-bold uppercase tracking-[0.15em] border-t border-border/50 flex items-center justify-center gap-3">
            <AlertTriangle size={14} className="animate-pulse" /> 
            {t('projectDetail.secrets.redeployNote')}
          </div>
        </Tabs>
      </Card>

      {/* dialog for Link SecretStore */}
      <Dialog open={isLinkModalOpen} onOpenChange={setIsLinkModalOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-base flex items-center gap-2">
              <Link2 className="w-4 h-4 text-primary" />
              Link SecretStore
            </DialogTitle>
            <DialogDescription className="text-xs">
              Bind a global secret container to this project environment.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleLinkStore} className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="link-store" className="text-xs font-bold uppercase tracking-wider text-muted-foreground">Select Store</Label>
              {linkableStores.length === 0 ? (
                <div className="text-xs text-muted-foreground py-2 border rounded-md px-3 bg-muted/10">
                  No other stores available. Create a new store in the Secret Store dashboard.
                </div>
              ) : (
                <Select
                  value={linkForm.storeId}
                  onValueChange={val => setLinkForm(prev => ({ ...prev, storeId: val || '' }))}
                >
                  <SelectTrigger id="link-store" className="h-9">
                    <SelectValue placeholder="Choose a SecretStore..." />
                  </SelectTrigger>
                  <SelectContent>
                    {linkableStores.map(store => (
                      <SelectItem key={store.id} value={String(store.id)}>
                        {store.name}
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
                <SelectTrigger id="link-env" className="h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t('secretstore.allEnvs')}</SelectItem>
                  <SelectItem value="production">{t('secretstore.prod')}</SelectItem>
                  <SelectItem value="staging">{t('secretstore.staging')}</SelectItem>
                  <SelectItem value="development">{t('secretstore.dev')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <DialogFooter className="pt-2">
              <Button type="button" variant="outline" size="sm" className="h-9 font-semibold uppercase tracking-wider text-[10px]" onClick={() => setIsLinkModalOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" disabled={!linkForm.storeId} className="h-9 px-6 font-semibold uppercase tracking-wider text-[10px]">
                Link Store
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
