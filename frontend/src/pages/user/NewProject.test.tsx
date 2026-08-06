import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { act } from 'react'
import NewProject from './NewProject'
import { githubAPI, databaseAPI, billingAPI } from '../../services/api'

vi.mock('../../services/api', () => ({
  projectsAPI: {
    create: vi.fn(),
  },
  githubAPI: {
    listInstallations: vi.fn(),
    listRepositories: vi.fn(),
    listBranches: vi.fn(),
    linkInstallation: vi.fn(),
  },
  databaseAPI: {
    listOwn: vi.fn(),
  },
  billingAPI: {
    catalog: vi.fn(),
  },
}))

const t = (key: string, data?: Record<string, unknown>) => {
  const map: Record<string, string> = {
    'projects.new.title': 'New Project',
    'projects.new.name': 'Name',
    'projects.new.description': 'Description',
    'projects.new.create': 'Create',
    'projects.new.billingPlan': 'Plan',
    'billing.projectPlan': 'Project Plan',
    'billing.databasePlan': 'Database Plan',
    'billing.creditsPerMonth': '{{credits}} credits/month',
    'billing.connections': '{{count}} connections',
    'billing.noPlans': 'No plans',
    'validation.required': 'Required',
  }
  const base = map[key] ?? key
  if (!data) return base
  return Object.entries(data).reduce((s, [k, v]) => s.replace(`{{${k}}}`, String(v)), base)
}

vi.mock('@/lib/useTranslation', () => ({
  default: () => ({ t, language: 'en' }),
}))

const mockSpecs = [
  { id: 1, type: 'project', name: 'Starter', monthly_credits: 100, cpu_millicores: 500, memory_mb: 512, connection_limit: 0 },
  { id: 2, type: 'project', name: 'Pro', monthly_credits: 500, cpu_millicores: 1000, memory_mb: 1024, connection_limit: 0 },
  { id: 3, type: 'database', name: 'DB Small', monthly_credits: 200, cpu_millicores: 250, memory_mb: 256, connection_limit: 10 },
]

describe('NewProject PlanSelector keyboard navigation', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('navigates project plan options with arrow keys and clicks', async () => {
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: { specs: mockSpecs } })
    ;(githubAPI.listRepositories as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })
    ;(databaseAPI.listOwn as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<NewProject />, { wrapper: MemoryRouter })

    await waitFor(() => expect(billingAPI.catalog).toHaveBeenCalledTimes(1))

    const buttons = screen.getAllByRole('radio')
    expect(buttons.length).toBeGreaterThanOrEqual(2)

    const first = buttons[0]
    const second = buttons[1]
    expect(first).toHaveAttribute('tabIndex', '0')
    expect(second).toHaveAttribute('tabIndex', '-1')

    await act(async () => fireEvent.keyDown(first, { key: 'ArrowDown' }))

    await waitFor(() => expect(second).toHaveAttribute('tabIndex', '0'))
    expect(second).toHaveAttribute('aria-checked', 'true')
  })
})
