import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { act } from 'react'
import AdminBilling from './Billing'
import { billingAPI } from '@/services/api'
import useAuthStore from '@/stores/authStore'

vi.mock('@/services/api', () => ({
  billingAPI: {
    adminCatalog: vi.fn(),
    adminWallets: vi.fn(),
    adminInvoices: vi.fn(),
    adminTopups: vi.fn(),
    adminSuspensions: vi.fn(),
    createSpec: vi.fn(),
    createTopupPackage: vi.fn(),
  },
  usersAPI: {
    list: vi.fn().mockResolvedValue({ data: [] }),
  },
}))

const t = (key: string, data?: Record<string, unknown>) => {
  const map: Record<string, string> = {
    'common.adminPanel': 'Admin',
    'billing.admin.title': 'Billing Admin',
    'billing.admin.description': 'Admin billing',
    'billing.admin.resourceType': 'Resource Type',
    'billing.admin.project': 'project',
    'billing.admin.database': 'database',
    'billing.admin.monthlyCredits': 'Monthly Credits',
    'billing.admin.name': 'Name',
    'billing.admin.slug': 'Slug',
    'billing.admin.cpu': 'CPU',
    'billing.admin.memory': 'Memory',
    'billing.admin.storage': 'Storage',
    'billing.admin.connectionLimit': 'Connection limit',
    'billing.admin.backupRetentionDays': 'Backup retention days',
    'billing.admin.reason': 'Reason',
    'billing.admin.createPlan': 'Create Plan',
    'billing.admin.create': 'Create',
    'billing.admin.versionedPricing': 'Versioned',
    'billing.admin.createSucceeded': 'Created',
    'billing.admin.createFailed': 'Failed',
    'billing.resourceTypes.project': 'project',
    'billing.resourceTypes.database': 'database',
    'billing.credits': 'credits',
    'billing.plans': 'Plans',
    'billing.admin.wallets': 'Wallets',
    'billing.invoices': 'Invoices',
    'billing.topups': 'Topups',
    'billing.admin.suspensions': 'Suspensions',
    'billing.admin.noRecords': 'No records',
    'billing.admin.total': '{{count}} of {{total}}',
    'billing.admin.user': 'User',
    'billing.retry': 'Retry',
    'billing.loadError': 'Load error',
    'billing.refresh': 'Refresh',
  }
  const base = map[key] ?? key
  if (!data) return base
  return Object.entries(data).reduce((s, [k, v]) => s.replace(`{{${k}}}`, String(v)), base)
}

vi.mock('@/lib/useTranslation', () => ({
  default: () => ({ t, language: 'en' }),
}))

vi.mock('@/stores/authStore', () => ({
  default: vi.fn(),
}))

const emptyPage = { page: 1, limit: 10, total: 0, data: [] }

describe('AdminBilling pricing form payload', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    ;(useAuthStore as unknown as ReturnType<typeof vi.fn>).mockReturnValue(true)
    ;(billingAPI.adminCatalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: { specs: [], packages: [] } })
    ;(billingAPI.adminWallets as ReturnType<typeof vi.fn>).mockResolvedValue({ data: emptyPage })
    ;(billingAPI.adminInvoices as ReturnType<typeof vi.fn>).mockResolvedValue({ data: emptyPage })
    ;(billingAPI.adminTopups as ReturnType<typeof vi.fn>).mockResolvedValue({ data: emptyPage })
    ;(billingAPI.adminSuspensions as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('submits project spec without database-only fields', async () => {
    ;(billingAPI.createSpec as ReturnType<typeof vi.fn>).mockResolvedValue({ data: {} })

    render(<AdminBilling />)
    await waitFor(() => expect(billingAPI.adminCatalog).toHaveBeenCalledTimes(1))

    fireEvent.change(screen.getByLabelText(/Name/i), { target: { value: 'Starter' } })
    fireEvent.change(screen.getByLabelText(/Slug/i), { target: { value: 'starter' } })
    fireEvent.change(screen.getByLabelText(/Monthly Credits/i), { target: { value: '100' } })
    fireEvent.change(screen.getByLabelText(/CPU/i), { target: { value: '500' } })
    fireEvent.change(screen.getByLabelText(/Memory/i), { target: { value: '512' } })
    fireEvent.change(screen.getByLabelText(/Storage/i), { target: { value: '10' } })
    fireEvent.change(screen.getAllByLabelText(/Reason/i)[0], { target: { value: 'init' } })

    await act(async () => fireEvent.click(screen.getAllByRole('button', { name: /Create/i })[0]))

    await waitFor(() => expect(billingAPI.createSpec).toHaveBeenCalledTimes(1))
    const payload = (billingAPI.createSpec as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(payload.type).toBe('project')
    expect(payload).not.toHaveProperty('connection_limit')
    expect(payload).not.toHaveProperty('backup_retention_days')
  })

  it('submits database spec with database fields', async () => {
    ;(billingAPI.createSpec as ReturnType<typeof vi.fn>).mockResolvedValue({ data: {} })

    render(<AdminBilling />)
    await waitFor(() => expect(billingAPI.adminCatalog).toHaveBeenCalledTimes(1))

    const hiddenInput = document.querySelector('input[name="type"]') as HTMLInputElement
    await act(async () => fireEvent.change(hiddenInput, { target: { value: 'database' } }))

    fireEvent.change(screen.getByLabelText(/Name/i), { target: { value: 'DB Small' } })
    fireEvent.change(screen.getByLabelText(/Slug/i), { target: { value: 'db-small' } })
    fireEvent.change(screen.getByLabelText(/Monthly Credits/i), { target: { value: '200' } })
    fireEvent.change(screen.getAllByLabelText(/Connection limit/i)[0], { target: { value: '10' } })
    fireEvent.change(screen.getAllByLabelText(/Backup retention days/i)[0], { target: { value: '7' } })
    fireEvent.change(screen.getAllByLabelText(/Reason/i)[0], { target: { value: 'init' } })

    await act(async () => fireEvent.click(screen.getAllByRole('button', { name: /Create/i })[0]))

    await waitFor(() => expect(billingAPI.createSpec).toHaveBeenCalledTimes(1))
    const payload = (billingAPI.createSpec as ReturnType<typeof vi.fn>).mock.calls[0][0]
    expect(payload.type).toBe('database')
    expect(payload.connection_limit).toBe(10)
    expect(payload.backup_retention_days).toBe(7)
  })
})
