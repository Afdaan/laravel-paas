import { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { secretStoreAPI } from '@/services/api'
import {
  FolderKey,
  Loader2,
  Search,
  User,
  Activity
} from 'lucide-react'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

interface AdminSecretStore {
  id: number
  user_id: number
  name: string
  description: string
  is_disabled: boolean
  created_at: string
  updated_at: string
  user: {
    name: string
    email: string
  }
  items?: Record<string, unknown>[]
  bindings?: Record<string, unknown>[]
}

interface SecretStoreActivityLog {
  id: number
  secret_store_id: number
  user_id: number
  project_id: number
  action: string
  ip_address: string
  user_agent: string
  details: string
  created_at: string
  user: {
    name: string
    email: string
  }
  project: {
    name: string
  }
}

export default function AdminSecretStoreExplorer() {
  const t = useCallback((key: string) => {
    const parts = key.split('.')
    if (parts[0] === 'secretstore' && parts[1] === 'title') return 'Secret Store Audit Explorer'
    if (parts[0] === 'secretstore' && parts[1] === 'desc') return 'Global governance console monitoring all encrypted credentials containers, bindings, and audit activities.'
    if (parts[0] === 'common') {
      if (parts[1] === 'loading') return 'Syncing database...'
      if (parts[1] === 'search') return 'Search...'
      if (parts[1] === 'name') return 'Name'
      if (parts[1] === 'status') return 'Status'
      if (parts[1] === 'date') return 'Date'
      if (parts[1] === 'actions') return 'Actions'
      if (parts[1] === 'error') return 'Operation failed'
    }
    return key
  }, [])

  const [stores, setStores] = useState<AdminSecretStore[]>([])
  const [logs, setLogs] = useState<SecretStoreActivityLog[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [searchQuery, setSearchQuery] = useState('')
  const [activeTab, setActiveTab] = useState('stores')

  const fetchData = useCallback(async () => {
    setIsLoading(true)
    try {
      const storesRes = await secretStoreAPI.adminListAll()
      const logsRes = await secretStoreAPI.adminListLogs()
      setStores(storesRes.data.data || [])
      setLogs(logsRes.data.data || [])
    } catch (error) {
      toast.error(t('common.error'))
    } finally {
      setIsLoading(false)
    }
  }, [t]);

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const handleToggleDisable = async (store: AdminSecretStore) => {
    try {
      await secretStoreAPI.adminDisable(store.id, !store.is_disabled)
      toast.success('Store status toggled successfully')
      fetchData()
    } catch (error) {
      toast.error(t('common.error'))
    }
  }

  const filteredStores = stores.filter(store =>
    store.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    (store.user?.name || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
    (store.user?.email || '').toLowerCase().includes(searchQuery.toLowerCase())
  )

  const filteredLogs = logs.filter(log =>
    log.action.toLowerCase().includes(searchQuery.toLowerCase()) ||
    (log.user?.name || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
    (log.details || '').toLowerCase().includes(searchQuery.toLowerCase())
  )

  return (
    <div className="space-y-6 animate-in fade-in duration-500 pb-10">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4 pb-4 border-b border-border/60">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-1 flex items-center gap-2">
            <FolderKey className="w-7 h-7 text-primary" />
            {t('secretstore.title')}
          </h1>
          <p className="text-sm text-muted-foreground">{t('secretstore.desc')}</p>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
        <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-4 pb-2 border-b border-border/60">
          <TabsList variant="line" className="bg-transparent h-10 p-0 gap-6">
            <TabsTrigger
              value="stores"
              className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent h-full px-0 font-bold uppercase tracking-widest text-[9px] text-muted-foreground data-[state=active]:text-foreground"
            >
              <FolderKey className="w-3.5 h-3.5 mr-1.5" />
              Credential Containers
            </TabsTrigger>
            <TabsTrigger
              value="logs"
              className="rounded-none border-b-2 border-transparent data-[state=active]:border-primary data-[state=active]:bg-transparent h-full px-0 font-bold uppercase tracking-widest text-[9px] text-muted-foreground data-[state=active]:text-foreground"
            >
              <Activity className="w-3.5 h-3.5 mr-1.5" />
              Global Access Audits
            </TabsTrigger>
          </TabsList>

          <div className="relative w-full md:max-w-xs">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
            <Input
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
              placeholder="Search stores, owners, actions..."
              className="pl-9 h-8 text-xs w-full"
            />
          </div>
        </div>

        {/* Containers Tab */}
        <TabsContent value="stores" className="mt-4 outline-none">
          {isLoading ? (
            <div className="flex justify-center py-20">
              <Loader2 className="w-8 h-8 animate-spin text-primary/50" />
            </div>
          ) : filteredStores.length === 0 ? (
            <div className="text-center py-20 border border-dashed border-border/50 rounded-lg text-xs text-muted-foreground">
              No credential containers match your search query.
            </div>
          ) : (
            <Card className="border-border/50 shadow-sm overflow-hidden">
              <Table>
                <TableHeader className="bg-muted/20">
                  <TableRow>
                    <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/4">Store Name</TableHead>
                    <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/4">Owner</TableHead>
                    <TableHead className="font-bold uppercase tracking-widest text-[9px] text-center">Keys</TableHead>
                    <TableHead className="font-bold uppercase tracking-widest text-[9px] text-center">Bindings</TableHead>
                    <TableHead className="font-bold uppercase tracking-widest text-[9px]">Created</TableHead>
                    <TableHead className="font-bold uppercase tracking-widest text-[9px] text-right">Governing Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredStores.map(store => (
                    <TableRow key={store.id} className="hover:bg-muted/10">
                      <TableCell className="font-bold text-xs text-foreground">
                        <div>
                          <p>{store.name}</p>
                          <p className="text-[10px] font-normal text-muted-foreground">{store.description || 'No description'}</p>
                        </div>
                      </TableCell>
                      <TableCell className="text-xs">
                        <div className="flex items-center gap-2">
                          <User className="w-3.5 h-3.5 text-muted-foreground" />
                          <div>
                            <p className="font-medium">{store.user?.name || 'Unknown'}</p>
                            <p className="text-[10px] text-muted-foreground">{store.user?.email || '-'}</p>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell className="text-center">
                        <Badge variant="outline" className="font-mono text-[10px] bg-muted/40 font-semibold">
                          {(store.items?.length || 0)} keys
                        </Badge>
                      </TableCell>
                      <TableCell className="text-center">
                        <Badge variant="outline" className="font-mono text-[10px] bg-muted/40 font-semibold">
                          {(store.bindings?.length || 0)} projects
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {new Date(store.created_at).toLocaleDateString()}
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex items-center justify-end gap-3">
                          <span className={`text-[10px] font-bold uppercase tracking-wider ${
                            store.is_disabled ? 'text-destructive' : 'text-emerald-500'
                          }`}>
                            {store.is_disabled ? 'Disabled' : 'Operational'}
                          </span>
                          <Switch
                            checked={!store.is_disabled}
                            onCheckedChange={() => handleToggleDisable(store)}
                          />
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          )}
        </TabsContent>

        {/* Logs Tab */}
        <TabsContent value="logs" className="mt-4 outline-none">
          {isLoading ? (
            <div className="flex justify-center py-20">
              <Loader2 className="w-8 h-8 animate-spin text-primary/50" />
            </div>
          ) : filteredLogs.length === 0 ? (
            <div className="text-center py-20 border border-dashed border-border/50 rounded-lg text-xs text-muted-foreground">
              No audit logs match your search query.
            </div>
          ) : (
            <Card className="border-border/50 shadow-sm overflow-hidden">
              <Table>
                <TableHeader className="bg-muted/20">
                  <TableRow>
                    <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/4">Operator</TableHead>
                    <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/5">Action</TableHead>
                    <TableHead className="font-bold uppercase tracking-widest text-[9px] w-1/3">Activity Details</TableHead>
                    <TableHead className="font-bold uppercase tracking-widest text-[9px]">Host Address & Client</TableHead>
                    <TableHead className="font-bold uppercase tracking-widest text-[9px] text-right">Timestamp</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredLogs.map(log => (
                    <TableRow key={log.id} className="hover:bg-muted/10">
                      <TableCell className="text-xs">
                        <div className="flex items-center gap-2">
                          <User className="w-3.5 h-3.5 text-muted-foreground" />
                          <div>
                            <p className="font-semibold">{log.user?.name || 'Unknown'}</p>
                            <p className="text-[10px] text-muted-foreground">{log.user?.email || '-'}</p>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant="outline"
                          className={`font-mono text-[9px] uppercase tracking-wider font-bold ${
                            log.action.includes('reveal')
                              ? 'text-rose-500 border-rose-500/20 bg-rose-500/5'
                              : log.action.includes('rotate') || log.action.includes('update')
                              ? 'text-amber-500 border-amber-500/20 bg-amber-500/5'
                              : 'text-blue-500 border-blue-500/20 bg-blue-500/5'
                          }`}
                        >
                          {log.action}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs text-foreground font-medium">
                        {log.details}
                        {log.project && (
                          <span className="text-[10px] text-muted-foreground block font-normal">
                            Bound Project: {log.project.name}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground max-w-[200px] truncate" title={`${log.ip_address} - ${log.user_agent}`}>
                        <p className="font-mono">{log.ip_address}</p>
                        <p className="text-[9px] truncate">{log.user_agent}</p>
                      </TableCell>
                      <TableCell className="text-right text-xs text-muted-foreground">
                        {new Date(log.created_at).toLocaleString()}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          )}
        </TabsContent>
      </Tabs>
    </div>
  )
}
