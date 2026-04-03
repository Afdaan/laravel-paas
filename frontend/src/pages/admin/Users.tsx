import React, { useState, useEffect, useRef, useCallback } from 'react'
import { toast } from 'sonner'
import { usersAPI } from '../../services/api'
import { User as UserType } from '../../types'
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
  CheckCircle2,
  Lock
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
  created: any[];
  errors: string[];
}

const AdminUsers = () => {
  const [users, setUsers] = useState<UserType[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
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
    role: 'student',
    password: '',
  })

  const fetchUsers = useCallback(async () => {
    setIsLoading(true)
    try {
      const roleQuery = roleFilter === 'all' ? '' : roleFilter
      const response = await usersAPI.list({ page, search, role: roleQuery, limit: 10 })
      setUsers(response.data.data || [])
      setTotal(response.data.total || 0)
    } catch (error) {
      toast.error('Failed to index users')
    } finally {
      setIsLoading(false)
    }
  }, [page, search, roleFilter])

  useEffect(() => {
    fetchUsers()
  }, [fetchUsers])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      if (editingUser) {
        await usersAPI.update(editingUser.id.toString(), formData)
        toast.success('Identity updated')
      } else {
        const response = await usersAPI.create(formData)
        toast.success(`Access provisioned! Pass: ${response.data.password}`)
      }
      setShowModal(false)
      setEditingUser(null)
      setFormData({ name: '', email: '', role: 'student', password: '' })
      fetchUsers()
    } catch (error: any) {
      toast.error(error.response?.data?.error || 'Authorization failed')
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
    if (!window.confirm('Purge this user identity? All associated projects remain but owner index is severed.')) return
    try {
      await usersAPI.delete(id.toString())
      toast.success('Identity purged')
      fetchUsers()
    } catch (error: any) {
      toast.error(error.response?.data?.error || 'Purge failed')
    }
  }

  const handleImport = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    
    try {
      const response = await usersAPI.importExcel(file)
      setImportResults(response.data)
      toast.success(`Imported ${response.data.total} identities`)
      fetchUsers()
    } catch (error: any) {
      toast.error(error.response?.data?.error || 'Manifest import failure')
    }
    
    e.target.value = ''
  }

  const totalPages = Math.ceil(total / 10)

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">Users Management</h1>
          <p className="text-muted-foreground">Manage all student and admin accounts across the platform.</p>
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
            Import CSV/Excel
          </Button>
          <Button 
            onClick={() => {
              setEditingUser(null)
              setFormData({ name: '', email: '', role: 'student', password: '' })
              setShowModal(true)
            }}
          >
            <UserPlus className="w-4 h-4 mr-2" />
            New User
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
                  <h3 className="text-lg font-bold">Sync Complete</h3>
                  <p className="text-sm font-medium text-emerald-600">{importResults.total} Identities successfully provisioned</p>
                </div>
              </div>
              <Button variant="ghost" size="icon" onClick={() => setImportResults(null)}>
                <X className="w-4 h-4" />
              </Button>
            </div>

            {importResults.errors?.length > 0 && (
              <div className="mb-4 p-4 rounded-md bg-destructive/10 border border-destructive/20 relative">
                <div className="flex items-center gap-2 mb-2 text-destructive font-semibold text-sm">
                  <AlertCircle className="w-4 h-4" /> Sync Anomalies
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
                      <TableHead>Identity</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Credential</TableHead>
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
                          <Badge variant="outline" className="text-emerald-500 border-emerald-500">Provisioned</Badge>
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
              placeholder="Search by name, email..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9 w-full md:max-w-md"
            />
          </div>
          <div className="w-full md:w-48">
            <Select value={roleFilter} onValueChange={(val) => setRoleFilter(val || 'all')}>
              <SelectTrigger>
                <SelectValue placeholder="Role: All Access" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Role: All Access</SelectItem>
                <SelectItem value="student">Level 1: Students</SelectItem>
                <SelectItem value="admin">Level 2: Internal Admin</SelectItem>
                <SelectItem value="superadmin">Level 3: Root</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>User Account</TableHead>
                <TableHead>Role / Access</TableHead>
                <TableHead>Created Date</TableHead>
                <TableHead className="text-right">Action</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={4} className="h-32 text-center text-muted-foreground font-medium uppercase tracking-widest text-xs">
                    Syncing Global Namespace
                  </TableCell>
                </TableRow>
              ) : users.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} className="h-32 text-center text-muted-foreground">
                    No users found matching parameters.
                  </TableCell>
                </TableRow>
              ) : users.map((user) => (
                <TableRow key={user.id}>
                  <TableCell>
                    <div className="flex items-center gap-4">
                      <div className={`w-10 h-10 rounded-full flex items-center justify-center ${
                          user.role === 'superadmin' ? 'bg-purple-500/10 text-purple-600' :
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
                  <TableCell>
                    <Badge variant="outline" className={`capitalize ${
                        user.role === 'superadmin' ? 'text-purple-600 border-purple-500/40 bg-purple-500/10' :
                        user.role === 'admin' ? 'text-indigo-600 border-indigo-500/40 bg-indigo-500/10' :
                        'text-emerald-600 border-emerald-500/40 bg-emerald-500/10'
                    }`}>
                      <Shield className="w-3 h-3 mr-1.5" />
                      {user.role}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2 text-sm text-muted-foreground">
                      <Calendar className="w-3.5 h-3.5" />
                      {new Date(user.created_at || '').toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
                    </div>
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-2">
                      <Button variant="ghost" size="icon" onClick={() => handleEdit(user)} title="Edit User">
                        <Edit3 className="w-4 h-4" />
                      </Button>
                      {user.role !== 'superadmin' && (
                        <Button variant="ghost" size="icon" className="text-destructive hover:bg-destructive/10 hover:text-destructive" onClick={() => handleDelete(user.id)} title="Delete User">
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

        {totalPages > 1 && (
          <div className="p-4 border-t flex flex-col sm:flex-row justify-between items-center gap-4 bg-muted/20">
            <span className="text-xs font-medium text-muted-foreground">
              Displaying {(page - 1) * 10 + 1} to {Math.min(page * 10, total)} of {total} identities.
            </span>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="icon" onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page === 1}>
                <ChevronLeft className="w-4 h-4" />
              </Button>
              <div className="text-sm font-semibold px-4 border rounded-md h-10 flex items-center justify-center min-w-[4rem]">
                {page} / {totalPages}
              </div>
              <Button variant="outline" size="icon" onClick={() => setPage(p => Math.min(totalPages, p + 1))} disabled={page === totalPages}>
                <ChevronRight className="w-4 h-4" />
              </Button>
            </div>
          </div>
        )}
      </Card>

      <Dialog open={showModal} onOpenChange={setShowModal}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle className="text-2xl uppercase tracking-tight">{editingUser ? 'Edit User' : 'Create User'}</DialogTitle>
            <DialogDescription>
              Configure the user's identity details and system privileges here.
            </DialogDescription>
          </DialogHeader>

          <form onSubmit={handleSubmit} className="space-y-6 mt-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>Full Name</Label>
                <Input
                  required
                  placeholder="eg. John Matrix"
                  value={formData.name}
                  onChange={(e) => setFormData(f => ({ ...f, name: e.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Email Address</Label>
                <Input
                  required
                  type="email"
                  placeholder="operator@system.io"
                  value={formData.email}
                  onChange={(e) => setFormData(f => ({ ...f, email: e.target.value }))}
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label>Access Privilege</Label>
              <div className="grid grid-cols-2 gap-4">
                <div 
                  className={`p-4 rounded-xl border cursor-pointer hover:bg-muted/50 transition-colors ${formData.role === 'student' ? 'border-primary ring-1 ring-primary' : ''}`}
                  onClick={() => setFormData(f => ({ ...f, role: 'student' }))}
                >
                  <div className="flex items-center justify-between mb-2">
                    <Users className="w-5 h-5 text-muted-foreground" />
                    {formData.role === 'student' && <CheckCircle2 className="w-4 h-4 text-primary" />}
                  </div>
                  <p className="font-semibold text-sm">Level 1: Student</p>
                </div>
                <div 
                  className={`p-4 rounded-xl border cursor-pointer hover:bg-muted/50 transition-colors ${formData.role === 'admin' ? 'border-primary ring-1 ring-primary' : ''}`}
                  onClick={() => setFormData(f => ({ ...f, role: 'admin' }))}
                >
                  <div className="flex items-center justify-between mb-2">
                    <Shield className="w-5 h-5 text-muted-foreground" />
                    {formData.role === 'admin' && <CheckCircle2 className="w-4 h-4 text-primary" />}
                  </div>
                  <p className="font-semibold text-sm">Level 2: Admin</p>
                </div>
              </div>
            </div>

            <div className="space-y-2">
              <Label>Security Credentials {editingUser && '(Optional Override)'}</Label>
              <div className="relative">
                <Lock className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
                <Input
                  type="password"
                  placeholder={editingUser ? 'Unchanged (Encrypted)' : 'Strong Password String'}
                  required={!editingUser}
                  value={formData.password}
                  onChange={(e) => setFormData(f => ({ ...f, password: e.target.value }))}
                  className="pl-10"
                />
              </div>
            </div>

            <DialogFooter className="pt-4 border-t">
              <Button type="button" variant="ghost" onClick={() => setShowModal(false)}>Cancel</Button>
              <Button type="submit">{editingUser ? 'Save Identity' : 'Create Identity'}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

export default AdminUsers

