// ===========================================
// Admin User Management Page
// ===========================================

import { useState, useEffect, useRef, useCallback } from 'react'
import toast from 'react-hot-toast'
import { usersAPI } from '../../services/api'
import { 
  Users, 
  UserPlus, 
  FileDown, 
  Search, 
  Filter, 
  Shield, 
  Mail, 
  Calendar, 
  Edit3, 
  Trash2, 
  X, 
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  MoreVertical,
  CheckCircle2,
  Lock,
  Download
} from 'lucide-react'

const AdminUsers = () => {
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

    const fetchUsers = useCallback(async () => {
        setIsLoading(true)
        try {
            const response = await usersAPI.list({ page, search, role: roleFilter, limit: 10 })
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

    const handleSubmit = async (e) => {
        e.preventDefault()
        try {
            if (editingUser) {
                await usersAPI.update(editingUser.id, formData)
                toast.success('Identity updated')
            } else {
                const response = await usersAPI.create(formData)
                toast.success(`Access provisioned! Pass: ${response.data.password}`)
            }
            setShowModal(false)
            setEditingUser(null)
            setFormData({ name: '', email: '', role: 'student', password: '' })
            fetchUsers()
        } catch (error) {
            toast.error(error.response?.data?.error || 'Authorization failed')
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
        if (!confirm('Purge this user identity? All associated projects remain but owner index is severed.')) return
        try {
            await usersAPI.delete(id)
            toast.success('Identity purged')
            fetchUsers()
        } catch (error) {
            toast.error(error.response?.data?.error || 'Purge failed')
        }
    }

    const handleImport = async (e) => {
        const file = e.target.files[0]
        if (!file) return
        
        try {
            const response = await usersAPI.importExcel(file)
            setImportResults(response.data)
            toast.success(`Imported ${response.data.total} identities`)
            fetchUsers()
        } catch (error) {
            toast.error(error.response?.data?.error || 'Manifest import failure')
        }
        
        e.target.value = ''
    }

    const totalPages = Math.ceil(total / 10)

    return (
        <div className="space-y-12 animate-pop-in relative h-full">
            {/* Background Glows */}
            <div className="absolute top-0 right-0 w-[55vw] h-[55vw] bg-indigo-600/5 blur-[150px] rounded-full pointer-events-none z-0"></div>

            <div className="relative z-10">
                {/* Header Area */}
                <div className="flex flex-col xl:flex-row xl:items-end justify-between mb-12 gap-8">
                    <div>
                        <h1 className="text-5xl font-black text-white tracking-tighter mb-4 italic text-transparent bg-clip-text pr-2 pb-1 bg-gradient-to-r from-indigo-400 to-purple-400">Users</h1>
                        <p className="text-slate-400 text-lg font-medium">Manage all student and admin accounts across the platform.</p>
                    </div>

                    <div className="flex items-center gap-4">
                        <input
                            type="file"
                            ref={fileInputRef}
                            onChange={handleImport}
                            accept=".xlsx,.xls"
                            className="hidden"
                        />
                        <button 
                            onClick={() => fileInputRef.current?.click()}
                            className="btn btn-secondary py-3 px-6 text-sm font-black uppercase tracking-widest active:scale-95 transition-all"
                        >
                            <FileDown className="w-4 h-4" />
                            Import List
                        </button>
                        <button 
                            onClick={() => {
                                setEditingUser(null)
                                setFormData({ name: '', email: '', role: 'student', password: '' })
                                setShowModal(true)
                            }}
                            className="btn btn-primary py-3 px-6 text-sm font-black uppercase tracking-widest active:scale-95 transition-all shadow-[0_10px_20px_rgba(99,102,241,0.2)]"
                        >
                            <UserPlus className="w-4 h-4" />
                            New User
                        </button>
                    </div>
                </div>

                {/* Import Results Area */}
                {importResults && (
                    <div className="mb-10 card-glass p-8 bg-emerald-500/5 border-emerald-500/20 overflow-hidden relative group">
                        <div className="absolute top-0 right-0 w-64 h-64 bg-emerald-500/10 blur-[80px] pointer-events-none" />
                        
                        <div className="relative flex justify-between items-start mb-8">
                            <div className="flex items-center gap-4">
                                <div className="w-12 h-12 rounded-2xl bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400 shadow-[0_0_20px_rgba(16,185,129,0.2)]">
                                    <CheckCircle2 className="w-6 h-6" />
                                </div>
                                <div>
                                    <h3 className="text-xl font-black text-white uppercase tracking-tight">Sync Complete</h3>
                                    <p className="text-emerald-400/80 font-bold text-sm tracking-wide">{importResults.total} Identities successfully provisioned</p>
                                </div>
                            </div>
                            <button onClick={() => setImportResults(null)} className="w-10 h-10 flex items-center justify-center rounded-xl hover:bg-white/5 text-slate-500 hover:text-white transition-all">
                                <X className="w-5 h-5" />
                            </button>
                        </div>

                        {importResults.errors?.length > 0 && (
                            <div className="mb-6 p-4 rounded-2xl bg-rose-500/5 border border-rose-500/10">
                                <div className="flex items-center gap-3 mb-2">
                                    <AlertCircle className="w-4 h-4 text-rose-500" />
                                    <span className="text-[10px] font-black uppercase tracking-widest text-rose-400">Sync Anomalies</span>
                                </div>
                                <ul className="space-y-1 pl-7">
                                    {importResults.errors.map((err, i) => <li key={i} className="text-xs text-rose-400/70 font-medium">{err}</li>)}
                                </ul>
                            </div>
                        )}

                        {importResults.created?.length > 0 && (
                            <div className="rounded-2xl border border-white/5 bg-black/20 overflow-hidden">
                                <div className="max-h-64 overflow-auto scrollbar-thin scrollbar-thumb-white/10">
                                    <table className="w-full text-left">
                                        <thead>
                                            <tr className="border-b border-white/5">
                                                <th className="px-6 py-4 text-[9px] font-black uppercase tracking-widest text-slate-500">Identity</th>
                                                <th className="px-6 py-4 text-[9px] font-black uppercase tracking-widest text-slate-500">Status</th>
                                                <th className="px-6 py-4 text-[9px] font-black uppercase tracking-widest text-slate-500">Credential</th>
                                            </tr>
                                        </thead>
                                        <tbody className="divide-y divide-white/[0.03]">
                                            {importResults.created.map((u, i) => (
                                                <tr key={i} className="hover:bg-white/[0.02] transition-all">
                                                    <td className="px-6 py-4">
                                                        <div className="flex flex-col">
                                                            <span className="text-xs font-black text-white">{u.name}</span>
                                                            <span className="text-[10px] text-slate-500 font-medium tracking-tight overflow-hidden text-ellipsis max-w-[150px]">{u.email}</span>
                                                        </div>
                                                    </td>
                                                    <td className="px-6 py-4">
                                                        <span className="text-[9px] font-black uppercase tracking-widest text-emerald-500/70">Provisioned</span>
                                                    </td>
                                                    <td className="px-6 py-4">
                                                        <div className="flex items-center gap-2 group/pass">
                                                             <code className="text-[11px] font-mono text-indigo-400 bg-indigo-500/5 px-2 py-1 rounded border border-indigo-500/10">{u.password}</code>
                                                        </div>
                                                    </td>
                                                </tr>
                                            ))}
                                        </tbody>
                                    </table>
                                </div>
                            </div>
                        )}
                    </div>
                )}

                {/* Toolbar */}
                <div className="flex flex-col md:flex-row items-center justify-between mb-8 gap-6 bg-white/[0.02] border border-white/10 p-4 rounded-3xl backdrop-blur-md shadow-2xl">
                    <div className="flex items-center gap-4 flex-1 w-full max-w-2xl">
                        <div className="relative flex-1 group">
                            <div className="absolute inset-y-0 left-0 pl-5 flex items-center pointer-events-none">
                                <Search className="w-4 h-4 text-slate-500 group-focus-within:text-indigo-400 transition-colors" />
                            </div>
                            <input 
                                type="text"
                                placeholder="Search by name, email, or access token..."
                                value={search}
                                onChange={(e) => setSearch(e.target.value)}
                                className="w-full bg-black/40 border border-white/5 rounded-2xl py-3.5 pl-12 pr-5 text-sm font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all placeholder:text-slate-600 outline-none"
                            />
                        </div>
                    </div>

                    <div className="flex items-center gap-4">
                        <select
                            value={roleFilter}
                            onChange={(e) => setRoleFilter(e.target.value)}
                            className="bg-black/40 border border-white/10 rounded-2xl px-6 py-3.5 text-[10px] font-black uppercase tracking-widest text-slate-400 outline-none focus:border-indigo-500 transition-all cursor-pointer"
                        >
                            <option value="">Role: All Access</option>
                            <option value="student">Level 1: Students</option>
                            <option value="admin">Level 2: Internal Admin</option>
                            <option value="superadmin">Level 3: Root Operator</option>
                        </select>
                    </div>
                </div>

                {/* Users Table Area */}
                <div className="card-glass overflow-hidden border-white/10 shadow-[0_30px_60px_rgba(0,0,0,0.4)] bg-white/[0.01]">
                    <div className="overflow-x-auto">
                        <table className="premium-table">
                            <thead>
                                <tr>
                                    <th>User Account</th>
                                    <th>Role / Access</th>
                                    <th>Created Date</th>
                                    <th className="text-right">Action</th>
                                </tr>
                            </thead>
                            <tbody>
                                {isLoading ? (
                                    <tr>
                                        <td colSpan="4" className="py-24 text-center">
                                            <div className="flex flex-col items-center gap-6">
                                                <div className="w-12 h-12 border-4 border-indigo-500/10 border-t-indigo-500 rounded-full animate-spin"></div>
                                                <p className="text-[10px] font-black uppercase tracking-[0.3em] text-slate-600 animate-pulse">Syncing Global Namespace</p>
                                            </div>
                                        </td>
                                    </tr>
                                ) : users.map((user) => (
                                    <tr key={user.id} className="group hover:bg-white/[0.03]">
                                        <td>
                                            <div className="flex items-center gap-5">
                                                <div className={`w-12 h-12 rounded-2xl border flex items-center justify-center transition-all duration-500 relative ${
                                                    user.role === 'superadmin' ? 'border-purple-500/40 bg-purple-500/5 text-purple-400 shadow-[0_10px_20px_rgba(168,85,247,0.1)]' :
                                                    user.role === 'admin' ? 'border-indigo-500/40 bg-indigo-500/5 text-indigo-400 shadow-[0_10px_20px_rgba(99,102,241,0.1)]' :
                                                    'border-emerald-500/40 bg-emerald-500/5 text-emerald-400 shadow-[0_10px_20px_rgba(16,185,129,0.1)]'
                                                }`}>
                                                    <Users className="w-6 h-6" />
                                                </div>
                                                <div className="flex flex-col">
                                                    <span className="text-sm font-black text-white group-hover:text-indigo-400 transition-colors uppercase tracking-tight">{user.name}</span>
                                                    <div className="flex items-center gap-2 mt-1">
                                                        <Mail className="w-3 h-3 text-slate-600" />
                                                        <p className="text-[10px] text-slate-500 font-medium tracking-tight overflow-hidden text-ellipsis max-w-[200px]">{user.email}</p>
                                                    </div>
                                                </div>
                                            </div>
                                        </td>
                                        <td>
                                            <span className={`px-4 py-1.5 rounded-full border text-[9px] font-black uppercase tracking-widest inline-flex items-center gap-2 ${
                                                user.role === 'superadmin' ? 'bg-purple-500/10 text-purple-400 border-purple-500/20 shadow-[0_0_15px_rgba(168,85,247,0.1)]' :
                                                user.role === 'admin' ? 'bg-indigo-500/10 text-indigo-400 border-indigo-500/20 shadow-[0_0_15px_rgba(99,102,241,0.1)]' :
                                                'bg-emerald-500/10 text-emerald-400 border-emerald-500/20 shadow-[0_0_15px_rgba(16,185,129,0.1)]'
                                            }`}>
                                                <Shield className="w-3 h-3" />
                                                {user.role}
                                            </span>
                                        </td>
                                        <td>
                                            <div className="flex items-center gap-3 text-slate-500 group-hover:text-slate-300 transition-colors">
                                                <Calendar className="w-3.5 h-3.5 opacity-50" />
                                                <span className="text-[11px] font-black uppercase tracking-widest">
                                                    {new Date(user.created_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
                                                </span>
                                            </div>
                                        </td>
                                        <td className="text-right">
                                            <div className="flex items-center justify-end gap-2">
                                                <button 
                                                    onClick={() => handleEdit(user)}
                                                    className="w-10 h-10 flex items-center justify-center rounded-xl bg-white/[0.02] border border-white/5 text-slate-500 hover:text-white hover:bg-white/[0.05] transition-all active:scale-90"
                                                    title="Modify Index"
                                                >
                                                    <Edit3 size={16} />
                                                </button>
                                                {user.role !== 'superadmin' && (
                                                    <button 
                                                        onClick={() => handleDelete(user.id)}
                                                        className="w-10 h-10 flex items-center justify-center rounded-xl bg-rose-500/5 border border-rose-500/10 text-rose-500/40 hover:text-rose-500 hover:bg-rose-500/10 transition-all active:scale-90"
                                                        title="Purge Record"
                                                    >
                                                        <Trash2 size={16} />
                                                    </button>
                                                )}
                                            </div>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>

                    {/* Pagination */}
                    {totalPages > 1 && (
                        <div className="p-10 border-t border-white/5 bg-white/[0.01] flex flex-col md:flex-row justify-between items-center gap-8">
                            <div className="flex items-center gap-3">
                                <Users className="w-4 h-4 text-indigo-500" />
                                <span className="text-[10px] font-black text-slate-500 uppercase tracking-widest">Displaying {(page - 1) * 10 + 1} to {Math.min(page * 10, total)} of {total} global identities.</span>
                            </div>
                            <div className="flex items-center gap-3 bg-black/40 border border-white/5 p-1.5 rounded-2xl">
                                <button
                                    onClick={() => setPage(p => Math.max(1, p - 1))}
                                    disabled={page === 1}
                                    className="w-12 h-12 flex items-center justify-center rounded-xl bg-white/[0.02] border border-white/5 text-white hover:bg-white/[0.05] disabled:opacity-20 disabled:cursor-not-allowed transition-all"
                                >
                                    <ChevronLeft size={20} />
                                </button>
                                <div className="px-6 font-mono text-xs font-black text-indigo-400">
                                    {page} <span className="text-slate-700 mx-2">/</span> {totalPages}
                                </div>
                                <button
                                    onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                                    disabled={page === totalPages}
                                    className="w-12 h-12 flex items-center justify-center rounded-xl bg-white/[0.02] border border-white/5 text-white hover:bg-white/[0.05] disabled:opacity-20 disabled:cursor-not-allowed transition-all"
                                >
                                    <ChevronRight size={20} />
                                </button>
                            </div>
                        </div>
                    )}
                </div>
            </div>

            {/* Modal - Overhaul to Luxury Glassmorphic */}
            {showModal && (
                <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 p-6 animate-fade-in transition-opacity duration-300">
                    <div className="absolute inset-0 z-0 bg-indigo-500/5 blur-[120px] pointer-events-none" />
                    
                    <div className="card-glass w-full max-w-xl bg-black/80 border-white/10 shadow-[0_50px_100px_rgba(0,0,0,0.8)] overflow-hidden relative z-10 animate-scale-up">
                        <div className="absolute top-0 right-0 w-64 h-64 bg-indigo-500/10 blur-[80px] pointer-events-none" />
                        
                        <div className="p-10">
                            <div className="flex items-center justify-between mb-10">
                                <div className="flex items-center gap-5">
                                    <div className="w-14 h-14 bg-indigo-500/10 border border-indigo-500/20 rounded-[1.25rem] flex items-center justify-center text-indigo-400 shadow-[0_0_25px_rgba(99,102,241,0.25)]">
                                        <UserPlus className="w-7 h-7" />
                                    </div>
                                    <div>
                                        <h2 className="text-3xl font-black text-white tracking-tighter uppercase">
                                            {editingUser ? 'Edit User' : 'Create User'}
                                        </h2>
                                        <p className="text-xs text-slate-500 font-bold tracking-widest uppercase mt-1">User Account Settings</p>
                                    </div>
                                </div>
                                <button onClick={() => setShowModal(false)} className="w-12 h-12 flex items-center justify-center rounded-2xl bg-white/[0.02] border border-white/10 text-slate-500 hover:text-white transition-all active:scale-95">
                                    <X size={24} />
                                </button>
                            </div>

                            <form onSubmit={handleSubmit} className="space-y-8">
                                <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                                    <div className="space-y-3">
                                        <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1">Full Identity Name</label>
                                        <input
                                            type="text"
                                            value={formData.name}
                                            onChange={(e) => setFormData(f => ({ ...f, name: e.target.value }))}
                                            className="w-full bg-black/40 border border-white/10 rounded-2xl px-5 py-4 text-white font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all"
                                            placeholder="eg. John Matrix"
                                            required
                                        />
                                    </div>
                                    <div className="space-y-3">
                                        <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1">System Email Address</label>
                                        <input
                                            type="email"
                                            value={formData.email}
                                            onChange={(e) => setFormData(f => ({ ...f, email: e.target.value }))}
                                            className="w-full bg-black/40 border border-white/10 rounded-2xl px-5 py-4 text-white font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all"
                                            placeholder="operator@afdaan.io"
                                            required
                                        />
                                    </div>
                                </div>

                                <div className="space-y-3">
                                    <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1">Access Level Privilege</label>
                                    <div className="grid grid-cols-2 gap-4">
                                        <button 
                                            type="button"
                                            onClick={() => setFormData(f => ({ ...f, role: 'student' }))}
                                            className={`p-4 rounded-2xl border text-left transition-all ${
                                                formData.role === 'student' 
                                                ? 'bg-emerald-500/10 border-emerald-500/40' 
                                                : 'bg-white/[0.02] border-white/5 hover:bg-white/[0.05]'
                                            }`}
                                        >
                                            <div className="flex items-center justify-between mb-2">
                                                <Users className={`w-5 h-5 ${formData.role === 'student' ? 'text-emerald-400' : 'text-slate-600'}`} />
                                                {formData.role === 'student' && <CheckCircle2 className="w-4 h-4 text-emerald-400" />}
                                            </div>
                                            <p className={`text-xs font-black uppercase tracking-tight ${formData.role === 'student' ? 'text-white' : 'text-slate-500'}`}>Level 1: Student</p>
                                        </button>
                                        <button 
                                            type="button"
                                            onClick={() => setFormData(f => ({ ...f, role: 'admin' }))}
                                            className={`p-4 rounded-2xl border text-left transition-all ${
                                                formData.role === 'admin' 
                                                ? 'bg-indigo-500/10 border-indigo-500/40' 
                                                : 'bg-white/[0.02] border-white/5 hover:bg-white/[0.05]'
                                            }`}
                                        >
                                            <div className="flex items-center justify-between mb-2">
                                                <Shield className={`w-5 h-5 ${formData.role === 'admin' ? 'text-indigo-400' : 'text-slate-600'}`} />
                                                {formData.role === 'admin' && <CheckCircle2 className="w-4 h-4 text-indigo-400" />}
                                            </div>
                                            <p className={`text-xs font-black uppercase tracking-tight ${formData.role === 'admin' ? 'text-white' : 'text-slate-500'}`}>Level 2: Admin</p>
                                        </button>
                                    </div>
                                </div>

                                <div className="space-y-3">
                                    <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1">
                                        Security Credentials {editingUser && '(Optional Override)'}
                                    </label>
                                    <div className="relative group">
                                        <div className="absolute inset-y-0 left-0 pl-5 flex items-center pointer-events-none">
                                            <Lock className="w-4 h-4 text-slate-600 group-focus-within:text-indigo-400" />
                                        </div>
                                        <input
                                            type="password"
                                            value={formData.password}
                                            onChange={(e) => setFormData(f => ({ ...f, password: e.target.value }))}
                                            className="w-full bg-black/40 border border-white/10 rounded-2xl py-4 pl-12 pr-5 text-white font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all outline-none"
                                            placeholder={editingUser ? 'Unchanged (System Encrypted)' : 'Strong Cluster Key'}
                                            required={!editingUser}
                                        />
                                    </div>
                                </div>

                                <div className="flex gap-4 pt-4">
                                    <button 
                                        type="button" 
                                        onClick={() => setShowModal(false)} 
                                        className="btn btn-secondary flex-1 py-4 text-sm font-black uppercase tracking-widest active:scale-95 transition-all"
                                    >
                                        Abort
                                    </button>
                                    <button 
                                        type="submit" 
                                        className="btn btn-primary flex-1 py-4 text-sm font-black uppercase tracking-widest active:scale-95 transition-all shadow-[0_15px_30px_rgba(99,102,241,0.3)]"
                                    >
                                        {editingUser ? 'Save Changes' : 'Create User'}
                                    </button>
                                </div>
                            </form>
                        </div>
                    </div>
                </div>
            )}
        </div>
    )
}

export default AdminUsers

