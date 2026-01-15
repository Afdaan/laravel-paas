// ===========================================
// Admin Users Page
// ===========================================

import { useState, useEffect, useRef } from 'react'
import toast from 'react-hot-toast'
import { usersAPI } from '../../services/api'

function AdminUsers() {
  const [users, setUsers] = useState([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [roleFilter, setRoleFilter] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [showModal, setShowModal] = useState(false)
  const [editingUser, setEditingUser] = useState(null)
  const [importResults, setImportResults] = useState(null)
  const fileInputRef = useRef(null)
  
  const [formData, setFormData] = useState({
    name: '',
    email: '',
    role: 'student',
    password: '',
  })
  
  useEffect(() => {
    fetchUsers()
  }, [page, search, roleFilter])
  
  const fetchUsers = async () => {
    setIsLoading(true)
    try {
      const response = await usersAPI.list({ page, search, role: roleFilter, limit: 10 })
      setUsers(response.data.data || [])
      setTotal(response.data.total || 0)
    } catch (error) {
      toast.error('Failed to fetch users')
    } finally {
      setIsLoading(false)
    }
  }
  
  const handleSubmit = async (e) => {
    e.preventDefault()
    try {
      if (editingUser) {
        await usersAPI.update(editingUser.id, formData)
        toast.success('User updated')
      } else {
        const response = await usersAPI.create(formData)
        toast.success(`User created! Password: ${response.data.password}`)
      }
      setShowModal(false)
      setEditingUser(null)
      setFormData({ name: '', email: '', role: 'student', password: '' })
      fetchUsers()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Operation failed')
    }
  }
  
  const handleEdit = (user) => {
    setEditingUser(user)
    setFormData({
      name: user.name,
      email: user.email,
      role: user.role,
      password: '',
    })
    setShowModal(true)
  }
  
  const handleDelete = async (id) => {
    if (!confirm('Are you sure you want to delete this user?')) return
    try {
      await usersAPI.delete(id)
      toast.success('User deleted')
      fetchUsers()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Delete failed')
    }
  }
  
  const handleImport = async (e) => {
    const file = e.target.files[0]
    if (!file) return
    
    try {
      const response = await usersAPI.importExcel(file)
      setImportResults(response.data)
      toast.success(`Imported ${response.data.total} users`)
      fetchUsers()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Import failed')
    }
    
    e.target.value = ''
  }
  
  const totalPages = Math.ceil(total / 10)
  
  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">User Management</h1>
          <p className="text-slate-400">Manage students and administrators</p>
        </div>
        <div className="flex gap-2">
          <input
            type="file"
            ref={fileInputRef}
            onChange={handleImport}
            accept=".xlsx,.xls"
            className="hidden"
          />
          <button 
            onClick={() => fileInputRef.current?.click()}
            className="btn btn-secondary"
          >
            📥 Import Excel
          </button>
          <button 
            onClick={() => {
              setEditingUser(null)
              setFormData({ name: '', email: '', role: 'student', password: '' })
              setShowModal(true)
            }}
            className="btn btn-primary"
          >
            + Add User
          </button>
        </div>
      </div>
      
      {/* Import Results */}
      {importResults && (
        <div className="card p-4 bg-emerald-600/10 border-emerald-600/30">
          <div className="flex justify-between items-start">
            <div>
              <h3 className="text-emerald-400 font-semibold">Import Complete</h3>
              <p className="text-slate-300">{importResults.total} users imported</p>
              {importResults.errors?.length > 0 && (
                <ul className="text-sm text-red-400 mt-2">
                  {importResults.errors.map((err, i) => <li key={i}>{err}</li>)}
                </ul>
              )}
            </div>
            <button onClick={() => setImportResults(null)} className="text-slate-400 hover:text-white">
              ✕
            </button>
          </div>
          {importResults.created?.length > 0 && (
            <div className="mt-4 max-h-40 overflow-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr><th>Name</th><th>Email</th><th>Password</th></tr>
                </thead>
                <tbody>
                  {importResults.created.map((u, i) => (
                    <tr key={i}>
                      <td>{u.name}</td>
                      <td>{u.email}</td>
                      <td className="font-mono text-primary-400">{u.password}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
      
      {/* Filters */}
      <div className="flex gap-4">
        <input
          type="text"
          placeholder="Search by name or email..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="flex-1 px-4 py-2 border"
        />
        <select
          value={roleFilter}
          onChange={(e) => setRoleFilter(e.target.value)}
          className="px-4 py-2 border"
        >
          <option value="">All Roles</option>
          <option value="student">Students</option>
          <option value="admin">Admins</option>
          <option value="superadmin">Superadmin</option>
        </select>
      </div>
      
      {/* Users Table */}
      <div className="card overflow-hidden">
        {isLoading ? (
          <div className="p-12 text-center">
            <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-primary-500 mx-auto"></div>
          </div>
        ) : (
          <table>
            <thead>
              <tr className="border-b border-slate-700">
                <th>Name</th>
                <th>Email</th>
                <th>Role</th>
                <th>Created</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td className="font-medium text-white">{user.name}</td>
                  <td>{user.email}</td>
                  <td>
                    <span className={`badge ${
                      user.role === 'superadmin' ? 'bg-purple-500/20 text-purple-400' :
                      user.role === 'admin' ? 'bg-blue-500/20 text-blue-400' :
                      'bg-slate-500/20 text-slate-400'
                    }`}>
                      {user.role}
                    </span>
                  </td>
                  <td className="text-slate-400">
                    {new Date(user.created_at).toLocaleDateString()}
                  </td>
                  <td>
                    <div className="flex gap-2">
                      <button 
                        onClick={() => handleEdit(user)}
                        className="text-primary-400 hover:text-primary-300"
                      >
                        Edit
                      </button>
                      {user.role !== 'superadmin' && (
                        <button 
                          onClick={() => handleDelete(user.id)}
                          className="text-red-400 hover:text-red-300"
                        >
                          Delete
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        
        {/* Pagination */}
        {totalPages > 1 && (
          <div className="p-4 border-t border-slate-700 flex justify-between items-center">
            <p className="text-slate-400 text-sm">
              Showing {(page - 1) * 10 + 1} to {Math.min(page * 10, total)} of {total}
            </p>
            <div className="flex gap-2">
              <button
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page === 1}
                className="btn btn-secondary text-sm disabled:opacity-50"
              >
                Previous
              </button>
              <button
                onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                disabled={page === totalPages}
                className="btn btn-secondary text-sm disabled:opacity-50"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </div>
      
      {/* Modal */}
      {showModal && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="card p-6 w-full max-w-md">
            <h2 className="text-xl font-semibold text-white mb-4">
              {editingUser ? 'Edit User' : 'Add User'}
            </h2>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="block text-sm text-slate-300 mb-1">Name</label>
                <input
                  type="text"
                  value={formData.name}
                  onChange={(e) => setFormData(f => ({ ...f, name: e.target.value }))}
                  className="w-full px-4 py-2 border"
                  required
                />
              </div>
              <div>
                <label className="block text-sm text-slate-300 mb-1">Email</label>
                <input
                  type="email"
                  value={formData.email}
                  onChange={(e) => setFormData(f => ({ ...f, email: e.target.value }))}
                  className="w-full px-4 py-2 border"
                  required
                />
              </div>
              <div>
                <label className="block text-sm text-slate-300 mb-1">Role</label>
                <select
                  value={formData.role}
                  onChange={(e) => setFormData(f => ({ ...f, role: e.target.value }))}
                  className="w-full px-4 py-2 border"
                >
                  <option value="student">Student</option>
                  <option value="admin">Admin</option>
                </select>
              </div>
              <div>
                <label className="block text-sm text-slate-300 mb-1">
                  Password {editingUser && '(leave empty to keep current)'}
                </label>
                <input
                  type="password"
                  value={formData.password}
                  onChange={(e) => setFormData(f => ({ ...f, password: e.target.value }))}
                  className="w-full px-4 py-2 border"
                  required={!editingUser}
                />
              </div>
              <div className="flex gap-2 pt-2">
                <button type="button" onClick={() => setShowModal(false)} className="btn btn-secondary flex-1">
                  Cancel
                </button>
                <button type="submit" className="btn btn-primary flex-1">
                  {editingUser ? 'Update' : 'Create'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}

export default AdminUsers
