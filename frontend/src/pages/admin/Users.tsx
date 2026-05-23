import React, { useState, useEffect, useRef, useCallback } from 'react'
import { toast } from 'sonner'
import { usersAPI } from '../../services/api'
import { AxiosError } from 'axios'
import useTranslation from '../../lib/useTranslation'
import { useNavigate } from 'react-router-dom'
import { User as UserType } from '../../types'
import useAuthStore from '../../stores/authStore'
import {
  Users,
  UserPlus,
  FileDown,
  Search,
  Shield,
  Mail,
  Calendar,
  Edit3,
  Trash2,
  X,
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  CheckCircle2,
  Lock,
  Globe,
  Clock,
  Activity,
  Info,
  UserCheck
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Card, CardContent } from '@/components/ui/card'

interface ImportResults {
  total: number;
  created: UserType[];
  errors: string[];
}

// ===========================================
// Helpers
// ===========================================

/**
 * Formats a date string into a human-readable "time ago" format.
 * Relies on the provided translation function for i18n support.
 */
const formatTimeAgo = (dateStr: string | undefined, t: (key: string, data?: Record<string, string | number>) => string) => {
  if (!dateStr) return '-'
  
  const date = new Date(dateStr)
  const now = new Date()
  const diffInSeconds = Math.floor((now.getTime() - date.getTime()) / 1000)
  
  if (diffInSeconds < 60) return t('common.justNow')
  
  if (diffInSeconds < 3600) {
    return t('admin.users.minutesAgo', { count: Math.floor(diffInSeconds / 60) })
  }
  
  if (diffInSeconds < 86400) {
    return t('admin.users.hoursAgo', { count: Math.floor(diffInSeconds / 3600) })
  }
  
  if (diffInSeconds < 2592000) {
    return t('admin.users.daysAgo', { count: Math.floor(diffInSeconds / 86400) })
  }
  
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

const AdminUsers = () => {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { loginAsClient } = useAuthStore()
  const [users, setUsers] = useState<UserType[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(10)
  const [search, setSearch] = useState('')
  const [roleFilter, setRoleFilter] = useState('all')
  const [isLoading, setIsLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)
  const [editingUser, setEditingUser] = useState<UserType | null>(null)
  const [importResults, setImportResults] = useState<ImportResults | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const [formData, setFormData] = useState({
    name: '',
    email: '',
    role: 'user',
    password: '',
  })

  const fetchUsers = useCallback(async () => {
    setIsLoading(true)
    try {
      const roleQuery = roleFilter === 'all' ? '' : roleFilter
      const response = await usersAPI.list({ page, search, role: roleQuery, limit })
      setUsers(response.data.data || [])
      setTotal(response.data.total || 0)
    } catch (error) {
      toast.error(t('common.loadError'))
    } finally {
      setIsLoading(false)
    }
  }, [page, search, roleFilter, limit, t])

  useEffect(() => {
    fetchUsers()
  }, [fetchUsers])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      if (editingUser) {
        await usersAPI.update(editingUser.id.toString(), formData)
        toast.success(t('common.updateSuccess'))
      } else {
        const response = await usersAPI.create(formData)
        toast.success(t('admin.users.accessProvisioned', { password: response.data.password }))
      }
      setShowModal(false)
      setEditingUser(null)
      setFormData({ name: '', email: '', role: 'user', password: '' })
      fetchUsers()
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: string }>
      toast.error(axiosError.response?.data?.error || t('common.actionFailed'))
    }
  }

  const handleEdit = (user: UserType) => {
    setEditingUser(user)
    setFormData({
      name: user.name,
      email: user.email,
      role: user.role,
      password: '',
    })
    setShowModal(true)
  }

  const handleDelete = async (id: number) => {
    if (!window.confirm(t('admin.users.confirmPurge'))) return
    try {
      await usersAPI.delete(id.toString())
      toast.success(t('common.deleteSuccess'))
      fetchUsers()
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: string }>
      toast.error(axiosError.response?.data?.error || t('common.actionFailed'))
    }
  }

  const handleLoginAs = async (id: number) => {
    try {
      const response = await usersAPI.loginAs(id)
      const { token } = response.data
      await loginAsClient(token)
      toast.success('Successfully logged in as user')
      navigate('/dashboard')
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: string }>
      toast.error(axiosError.response?.data?.error || t('common.actionFailed'))
    }
  }

  const handleImport = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return

    try {
      const response = await usersAPI.importExcel(file)
      setImportResults(response.data)
      toast.success(t('admin.users.importSuccess', { count: response.data.total }))
      fetchUsers()
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: string }>
      toast.error(axiosError.response?.data?.error || t('common.actionFailed'))
    }

    e.target.value = ''
  }

  const totalPages = Math.ceil(total / limit)

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('admin.users.title')}</h1>
          <p className="text-muted-foreground">{t('admin.users.desc')}</p>
        </div>

        <div className="flex items-center gap-4">
          <input
            type="file"
            ref={fileInputRef}
            onChange={handleImport}
            accept=".xlsx,.xls"
            className="hidden"
          />
          <Button variant="outline" onClick={() => fileInputRef.current?.click()}>
            <FileDown className="w-4 h-4 mr-2" />
            {t('admin.users.importData')}
          </Button>
          <Button
            onClick={() => {
              setEditingUser(null)
              setFormData({ name: '', email: '', role: 'user', password: '' })
              setShowModal(true)
            }}
          >
            <UserPlus className="w-4 h-4 mr-2" />
            {t('admin.users.newUser')}
          </Button>
        </div>
      </div>

      {importResults && (
        <Card className="border-emerald-500/20 bg-emerald-500/5">
          <CardContent className="p-6 relative">
            <div className="flex justify-between items-start mb-6">
              <div className="flex items-center gap-4">
                <div className="w-10 h-10 rounded-full bg-emerald-500/20 text-emerald-600 flex items-center justify-center">
                  <CheckCircle2 className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-lg font-bold">{t('admin.users.syncComplete')}</h3>
                  <p className="text-sm font-medium text-emerald-600">{importResults.total} {t('admin.users.provisionSuccess')}</p>
                </div>
              </div>
              <Button variant="ghost" size="icon" onClick={() => setImportResults(null)}>
                <X className="w-4 h-4" />
              </Button>
            </div>

            {importResults.errors?.length > 0 && (
              <div className="mb-4 p-4 rounded-md bg-destructive/10 border border-destructive/20 relative">
                <div className="flex items-center gap-2 mb-2 text-destructive font-semibold text-sm">
                  <AlertCircle className="w-4 h-4" /> {t('admin.users.syncAnomalies')}
                </div>
                <ul className="list-disc pl-5 text-sm text-destructive/80 space-y-1">
                  {importResults.errors.map((err, i) => <li key={i}>{err}</li>)}
                </ul>
              </div>
            )}

            {importResults.created?.length > 0 && (
              <div className="border rounded-md max-h-64 overflow-auto bg-background/50">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('admin.users.identity')}</TableHead>
                      <TableHead>{t('common.status')}</TableHead>
                      <TableHead>{t('admin.users.credentialLabel')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {importResults.created.map((u, i) => (
                      <TableRow key={i}>
                        <TableCell>
                          <div className="font-semibold text-sm">{u.name}</div>
                          <div className="text-xs text-muted-foreground">{u.email}</div>
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline" className="text-emerald-500 border-emerald-500">{t('admin.users.provisionedBadge')}</Badge>
                        </TableCell>
                        <TableCell>
                          <code className="text-xs font-mono bg-secondary px-2 py-1 rounded">{u.password}</code>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      <Card>
        <div className="p-6 border-b flex flex-col md:flex-row items-center gap-4">
          <div className="relative flex-1 w-full">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t('admin.users.searchPlaceholder')}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9 w-full md:max-w-md"
            />
          </div>
          <div className="w-full md:w-48">
            <Select value={roleFilter} onValueChange={(val) => setRoleFilter(val || 'all')}>
              <SelectTrigger className={'w-full'}>
                <SelectValue placeholder={t('admin.users.allAccess')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('admin.users.allAccess')}</SelectItem>
                <SelectItem value="user">{t('admin.users.level1')}</SelectItem>
                <SelectItem value="admin">{t('admin.users.level2')}</SelectItem>
                <SelectItem value="superadmin">{t('admin.users.level3')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className="overflow-x-auto">
          <Table className="min-w-[1100px] table-fixed">
            <TableHeader>
              <TableRow className="bg-muted/20 hover:bg-muted/20">
                <TableHead className="h-12 w-[24%] pl-6 pr-4 text-xs font-semibold uppercase tracking-wider">{t('common.name')}</TableHead>
                <TableHead className="h-12 w-[12%] px-4 text-xs font-semibold uppercase tracking-wider">{t('admin.users.roleLabel')}</TableHead>
                <TableHead className="h-12 w-[14%] px-4 text-xs font-semibold uppercase tracking-wider">
                  <div className="flex items-center gap-2">
                    <Activity className="w-3.5 h-3.5" />
                    {t('admin.users.activityLabel')}
                  </div>
                </TableHead>
                <TableHead className="h-12 w-[16%] px-4 text-xs font-semibold uppercase tracking-wider">{t('admin.users.lastLoginLabel')}</TableHead>
                <TableHead className="h-12 w-[16%] px-4 text-xs font-semibold uppercase tracking-wider">{t('admin.users.accessFromLabel')}</TableHead>
                <TableHead className="h-12 w-[18%] pl-4 pr-6 text-right text-xs font-semibold uppercase tracking-wider">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-32 text-center text-muted-foreground font-medium uppercase tracking-widest text-xs">
                    {t('common.loading')}
                  </TableCell>
                </TableRow>
              ) : (!users || users.length === 0) ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-32 text-center text-muted-foreground">
                    {t('common.noData')}
                  </TableCell>
                </TableRow>
              ) : users.map((user) => (
                <TableRow key={user.id} className="hover:bg-muted/20">
                  <TableCell className="pl-6 pr-4 py-3">
                    <div className="flex items-center gap-4">
                      <div className={`w-10 h-10 rounded-full flex items-center justify-center ${user.role === 'superadmin' ? 'bg-purple-500/10 text-purple-600' :
                          user.role === 'admin' ? 'bg-indigo-500/10 text-indigo-600' :
                            'bg-emerald-500/10 text-emerald-600'
                        }`}>
                        <Users className="w-5 h-5" />
                      </div>
                      <div className="flex flex-col">
                        <span className="text-sm font-semibold">{user.name}</span>
                        <div className="flex items-center gap-1.5 mt-0.5 text-xs text-muted-foreground">
                          <Mail className="w-3 h-3" />
                          <span className="truncate max-w-[200px]">{user.email}</span>
                        </div>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="px-4 py-3">
                    <Badge variant="outline" className={`capitalize ${user.role === 'superadmin' ? 'text-purple-600 border-purple-500/40 bg-purple-500/10' :
                        user.role === 'admin' ? 'text-indigo-600 border-indigo-500/40 bg-indigo-500/10' :
                          'text-emerald-600 border-emerald-500/40 bg-emerald-500/10'
                      }`}>
                      <Shield className="w-3 h-3 mr-1.5" />
                      {user.role}
                    </Badge>
                  </TableCell>
                  <TableCell className="px-4 py-3">
                    <div className="flex items-center gap-2 text-xs font-semibold">
                      <div className={`w-2 h-2 rounded-full ${
                        user.last_activity && (new Date().getTime() - new Date(user.last_activity).getTime() < 300000) 
                        ? 'bg-emerald-500 animate-pulse' 
                        : 'bg-slate-300'
                      }`} />
                      <span className={user.last_activity && (new Date().getTime() - new Date(user.last_activity).getTime() < 300000) ? 'text-emerald-600' : 'text-slate-500'}>
                        {formatTimeAgo(user.last_activity, t)}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className="px-4 py-3">
                    <div className="flex flex-col gap-1">
                      <div className="flex items-center gap-1.5 text-xs font-medium">
                        <Clock className="w-3 h-3 text-muted-foreground" />
                        {user.last_login ? new Date(user.last_login).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '-'}
                      </div>
                      <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                        <Calendar className="w-3 h-3" />
                        {t('admin.users.joined')} {new Date(user.created_at || '').toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="px-4 py-3">
                    <div className="flex flex-col gap-1">
                      <div className="flex items-center gap-1.5 text-xs font-mono font-medium">
                        <Shield className="w-3 h-3 text-muted-foreground" />
                        {user.last_ip || '-'}
                      </div>
                      {user.last_location && (
                        <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                          <Globe className="w-3 h-3" />
                          {user.last_location}
                        </div>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="pl-4 pr-6 py-3 text-right">
                    <div className="flex items-center justify-end gap-2">
                      {user.role === 'user' && (
                        <Button variant="outline" size="sm" className="h-8 text-xs font-medium" onClick={() => handleLoginAs(user.id)}>
                          <UserCheck className="w-3.5 h-3.5 mr-1.5" />
                          {t('admin.users.loginAs')}
                        </Button>
                      )}
                      <Button variant="ghost" size="icon" onClick={() => handleEdit(user)} title={t('common.edit')}>
                        <Edit3 className="w-4 h-4" />
                      </Button>
                      {user.role !== 'superadmin' && (
                        <Button variant="ghost" size="icon" className="text-destructive hover:bg-destructive/10 hover:text-destructive" onClick={() => handleDelete(user.id)} title={t('common.delete')}>
                          <Trash2 className="w-4 h-4" />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>

        {totalPages > 0 && (
          <div className="p-4 border-t flex flex-col sm:flex-row justify-between items-center gap-4 bg-muted/10">
            <div className="flex items-center gap-6">
              <div className="flex items-center gap-2 text-sm text-muted-foreground font-medium">
                <Info className="w-4 h-4 text-primary" />
                {t('admin.users.paginationInfo', { start: (page - 1) * limit + 1, end: Math.min(page * limit, total), total })}
              </div>
              <div className="flex items-center space-x-2">
                <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{t('common.rowsPerPage') || 'Rows per page'}</p>
                <Select
                  value={limit.toString()}
                  onValueChange={(value) => {
                    setLimit(Number(value))
                    setPage(1)
                  }}
                >
                  <SelectTrigger size="sm" className="h-8 w-[82px] justify-between">
                    <SelectValue placeholder={limit} />
                  </SelectTrigger>
                  <SelectContent
                    side="top"
                    align="end"
                    className="min-w-[120px] max-h-[220px] rounded-lg p-1 shadow-lg"
                  >
                    {[10, 15, 20, 25, 30, 40, 50, 75, 100].map((pageSize) => (
                      <SelectItem
                        key={pageSize}
                        value={`${pageSize}`}
                        className="rounded-md py-1.5 px-2 text-sm"
                      >
                        {pageSize}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="flex items-center space-x-6 lg:space-x-8">
              <div className="flex w-[100px] items-center justify-center text-sm font-medium">
                Page {page} of {Math.max(1, totalPages)}
              </div>
              <div className="flex items-center space-x-2">
                <Button
                  variant="outline"
                  className="hidden h-8 w-8 p-0 lg:flex"
                  onClick={() => setPage(1)}
                  disabled={page === 1}
                >
                  <span className="sr-only">Go to first page</span>
                  <ChevronsLeft className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  className="h-8 w-8 p-0"
                  onClick={() => setPage(p => Math.max(1, p - 1))}
                  disabled={page === 1}
                >
                  <span className="sr-only">Go to previous page</span>
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  className="h-8 w-8 p-0"
                  onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                  disabled={page === totalPages || totalPages === 0}
                >
                  <span className="sr-only">Go to next page</span>
                  <ChevronRight className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  className="hidden h-8 w-8 p-0 lg:flex"
                  onClick={() => setPage(totalPages)}
                  disabled={page === totalPages || totalPages === 0}
                >
                  <span className="sr-only">Go to last page</span>
                  <ChevronsRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </div>
        )}

      </Card>

      <Dialog open={showModal} onOpenChange={setShowModal}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle className="text-2xl uppercase tracking-tight">{editingUser ? t('common.edit') : t('common.create')} {t('admin.users.identity')}</DialogTitle>
            <DialogDescription>
              {t('admin.users.modalDesc')}
            </DialogDescription>
          </DialogHeader>

          <form onSubmit={handleSubmit} className="space-y-6 mt-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>{t('common.name')}</Label>
                <Input
                  required
                  placeholder={t('admin.users.namePlaceholder')}
                  value={formData.name}
                  onChange={(e) => setFormData(f => ({ ...f, name: e.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label>{t('login.email')}</Label>
                <Input
                  required
                  type="email"
                  placeholder={t('admin.users.emailPlaceholder')}
                  value={formData.email}
                  onChange={(e) => setFormData(f => ({ ...f, email: e.target.value }))}
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label>{t('admin.users.privilege')}</Label>
              <div className="grid grid-cols-2 gap-4">
                <div
                  className={`p-4 rounded-xl border cursor-pointer hover:bg-muted/50 transition-colors ${formData.role === 'user' ? 'border-primary ring-1 ring-primary' : ''}`}
                  onClick={() => setFormData(f => ({ ...f, role: 'user' }))}
                >
                  <div className="flex items-center justify-between mb-2">
                    <Users className="w-5 h-5 text-muted-foreground" />
                    {formData.role === 'user' && <CheckCircle2 className="w-4 h-4 text-primary" />}
                  </div>
                  <p className="font-semibold text-sm">{t('admin.users.level1')}</p>
                </div>
                <div
                  className={`p-4 rounded-xl border cursor-pointer hover:bg-muted/50 transition-colors ${formData.role === 'admin' ? 'border-primary ring-1 ring-primary' : ''}`}
                  onClick={() => setFormData(f => ({ ...f, role: 'admin' }))}
                >
                  <div className="flex items-center justify-between mb-2">
                    <Shield className="w-5 h-5 text-muted-foreground" />
                    {formData.role === 'admin' && <CheckCircle2 className="w-4 h-4 text-primary" />}
                  </div>
                  <p className="font-semibold text-sm">{t('admin.users.level2')}</p>
                </div>
              </div>
            </div>

            <div className="space-y-2">
              <Label>{t('admin.users.security')} {editingUser && t('admin.users.passOverride')}</Label>
              <div className="relative">
                <Lock className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
                <Input
                  type="password"
                  placeholder={editingUser ? t('admin.users.unchanged') : t('admin.users.passPlaceholder')}
                  required={!editingUser}
                  value={formData.password}
                  onChange={(e) => setFormData(f => ({ ...f, password: e.target.value }))}
                  className="pl-10"
                />
              </div>
            </div>

            <DialogFooter className="pt-4 border-t">
              <Button type="button" variant="ghost" onClick={() => setShowModal(false)}>{t('common.cancel')}</Button>
              <Button type="submit">{editingUser ? t('common.save') : t('common.create')} {t('admin.users.identity')}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

export default AdminUsers
