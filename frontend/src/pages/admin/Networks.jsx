// ===========================================
// Admin Networks Page (Placeholder)
// ===========================================

import { memo } from 'react'
import { Link } from 'react-router-dom'

const AdminNetworks = () => {
    return (
        <div className="min-h-screen bg-[#0a0a0c] text-slate-200">
            <div className="mb-8">
                <h1 className="text-3xl font-bold text-white tracking-tight">Networks</h1>
                <p className="text-slate-500 mt-1">Docker network configurations and isolation</p>
            </div>

            <div className="bg-[#111114] border border-white/[0.03] rounded-2xl p-20 text-center shadow-2xl">
                <div className="w-16 h-16 bg-indigo-500/10 rounded-full flex items-center justify-center text-indigo-400 mx-auto mb-6">
                    <svg className="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                    </svg>
                </div>
                <h2 className="text-xl font-bold text-white mb-2">Network Manager Coming Soon</h2>
                <p className="text-slate-500 max-w-md mx-auto mb-8">Detailed network management for bridge, host, and overlay networks is being integrated into the PaaS control plane.</p>
                <Link to="/admin/dashboard" className="px-6 py-2 bg-indigo-500 hover:bg-indigo-600 text-white rounded-lg font-bold transition-all">Back to Dashboard</Link>
            </div>
        </div>
    )
}

export default memo(AdminNetworks)
