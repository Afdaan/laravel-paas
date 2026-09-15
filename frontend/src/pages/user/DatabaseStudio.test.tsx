import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import DatabaseStudio from './DatabaseStudio'
import { databaseAPI } from '../../services/api'

vi.mock('@/lib/useTranslation', () => ({
  default: () => ({ t: (key: string) => key }),
}))

vi.mock('../../services/api', () => ({
  databaseAPI: {
    getOverview: vi.fn(),
    getCredentials: vi.fn(),
    getSchema: vi.fn(),
    listBackups: vi.fn(),
    getMetrics: vi.fn(),
    updateStatus: vi.fn(),
  },
}))

vi.mock('@/components/database-studio/StudioDashboardTab', () => ({
  StudioDashboardTab: () => <div>Dashboard loaded</div>,
}))

vi.mock('@/components/database-studio/StudioTablesTab', () => ({
  StudioTablesTab: () => <div>Tables loaded</div>,
}))

vi.mock('@/components/database-studio/StudioStructureTab', () => ({
  StudioStructureTab: () => <div>Structure loaded</div>,
}))

vi.mock('@/components/database-studio/StudioQueryTab', () => ({
  StudioQueryTab: () => <div>Query loaded</div>,
}))

vi.mock('@/components/database-studio/StudioBackupsTab', () => ({
  StudioBackupsTab: () => <div>Backups loaded</div>,
}))

describe('DatabaseStudio', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('loads managed database metadata without revealing credentials', async () => {
    vi.mocked(databaseAPI.getOverview).mockResolvedValue({
      data: {
        engine: 'mysql',
        status: 'active',
        database: 'app_db',
        username: 'app_user',
        host: 'paas-mysql',
        port: 3306,
      },
    })
    vi.mocked(databaseAPI.getSchema).mockResolvedValue({ data: { tables: [] } })
    vi.mocked(databaseAPI.listBackups).mockResolvedValue({ data: { backups: [] } })
    vi.mocked(databaseAPI.getMetrics).mockResolvedValue({ data: {} })

    render(
      <MemoryRouter initialEntries={['/projects/project-1/database']}>
        <Routes>
          <Route path="/projects/:id/database" element={<DatabaseStudio />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('Dashboard loaded')).toBeInTheDocument())

    expect(databaseAPI.getOverview).toHaveBeenCalledWith('project-1')
    expect(databaseAPI.getCredentials).not.toHaveBeenCalled()
  })

  it('shows a retry state instead of blank fallback credentials after loading fails', async () => {
    vi.mocked(databaseAPI.getOverview).mockRejectedValue(new Error('offline'))

    render(
      <MemoryRouter initialEntries={['/projects/project-1/database']}>
        <Routes>
          <Route path="/projects/:id/database" element={<DatabaseStudio />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('databaseStudio.errors.connectFailed')).toBeInTheDocument())
    expect(screen.queryByText('Dashboard loaded')).not.toBeInTheDocument()
  })
})
