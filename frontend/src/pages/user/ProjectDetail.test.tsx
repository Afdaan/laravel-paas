import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import ProjectDetail from './ProjectDetail'
import { projectsAPI } from '../../services/api'
import type { Project } from '@/types'

vi.mock('@/lib/usePolling', () => ({ usePolling: vi.fn() }))
vi.mock('../../lib/usePolling', () => ({ usePolling: vi.fn() }))

vi.mock('../../components/project/detail/OverviewTab', () => ({ OverviewTab: () => <div data-testid="overview-tab" /> }))
vi.mock('../../components/project/detail/BuildTab', () => ({ BuildTab: () => <div data-testid="build-tab" /> }))
vi.mock('../../components/project/detail/DomainsTab', () => ({ DomainsTab: () => <div data-testid="domains-tab" /> }))
vi.mock('../../components/project/detail/LogsTab', () => ({ LogsTab: () => <div data-testid="logs-tab" /> }))
vi.mock('../../components/project/detail/SettingsTab', () => ({ SettingsTab: () => <div data-testid="settings-tab" /> }))
vi.mock('../../components/project/RuntimeTab', () => ({ RuntimeTab: () => <div data-testid="runtime-tab" /> }))
vi.mock('../../components/project/RedeployButton', () => ({ RedeployButton: () => <div data-testid="redeploy-button" /> }))
vi.mock('../../components/project/RestartButton', () => ({ RestartButton: () => <div data-testid="restart-button" /> }))
vi.mock('../../components/ConfirmationModal', () => ({ default: () => null }))

vi.mock('../../services/api', () => ({
  projectsAPI: {
    get: vi.fn(),
    logs: vi.fn().mockResolvedValue({ data: { logs: '' } }),
    stats: vi.fn().mockResolvedValue({ data: { cpu_percent: 0, memory_usage: 0, memory_limit: 100 } }),
    getDeploymentEvents: vi.fn().mockResolvedValue({ data: [] }),
    listBranches: vi.fn().mockResolvedValue({ data: ['main'] }),
    rollback: vi.fn(),
    stop: vi.fn(),
    start: vi.fn(),
    redeploy: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
  },
  githubAPI: {
    listInstallations: vi.fn().mockResolvedValue({ data: { installations: [] } }),
    listRepositories: vi.fn().mockResolvedValue({ data: { repositories: [] } }),
    listBranches: vi.fn().mockResolvedValue({ data: { branches: [] } }),
  },
  getCSRFToken: vi.fn().mockReturnValue('mock-csrf'),
}))

const { mockT } = vi.hoisted(() => {
  const messages: Record<string, string> = {
    'common.loading': 'Loading...',
    'common.close': 'Close',
    'common.general': 'General',
    'projectDetail.messages.creationProjectFallback': 'your project',
    'projectDetail.messages.creationCreated': 'Project created',
    'projectDetail.messages.creationPreparingTitle': 'Preparing {{name}}',
    'projectDetail.messages.creationPreparingDesc': 'Project saved and deployment starting.',
    'projectDetail.messages.creationDeployment': 'Initial deployment',
    'projectDetail.messages.creationReady': 'Application ready',
    'projectDetail.messages.creationSuccessTitle': 'Project created successfully',
    'projectDetail.messages.creationSuccessDesc': 'The first deployment is still running. Follow its live progress below.',
    'projectDetail.messages.creationReadyTitle': 'Project is ready',
    'projectDetail.messages.creationReadyDesc': 'The first deployment completed and your application is online.',
    'projectDetail.messages.creationFailedTitle': 'Project created, deployment needs attention',
    'projectDetail.messages.creationFailedDesc': 'The project is saved, but the first deployment failed. Open Build Logs to inspect the error.',
    'projectDetail.overview.deployError': 'Deployment Error',
    'projectDetail.messages.failedDesc': 'Failed to deploy.',
    'projectDetail.tabs.overview': 'Overview',
    'projectDetail.tabs.build': 'Build Logs',
    'projectDetail.actions.stop': 'Stop',
    'projectDetail.actions.start': 'Start',
    'projectDetail.metrics.cpu': 'CPU',
    'projectDetail.metrics.load': 'Load',
    'projectDetail.metrics.managedStack': 'Managed Stack',
  }

  return {
    mockT: (key: string, data?: Record<string, unknown>) => {
      let value = messages[key] || key
      Object.entries(data || {}).forEach(([name, replacement]) => {
        value = value.replace(`{{${name}}}`, String(replacement))
      })
      return value
    },
  }
})

vi.mock('@/lib/useTranslation', () => ({
  default: () => ({ language: 'en', setLanguage: vi.fn(), t: mockT }),
}))

vi.mock('../../lib/useTranslation', () => ({
  default: () => ({ language: 'en', setLanguage: vi.fn(), t: mockT }),
}))

function createMockProject(overrides: Partial<Project> = {}): Project {
  return {
    id: 1,
    uid: 'proj-123',
    user_id: 1,
    name: 'billing-service',
    repository_url: 'https://github.com/example/billing-service',
    framework: 'Node.js',
    status: 'running',
    deployment_status: 'completed',
    subdomain: 'billing-service',
    url: 'https://billing-service.example.com',
    branch: 'main',
    php_version: '8.2',
    port: null,
    db_name: 'billing_db',
    created_at: '2026-09-20T10:00:00Z',
    ...overrides,
  }
}

describe('ProjectDetail creation banner outcomes', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders attention/failed banner when project creation deployment status is cancelled', async () => {
    const cancelledProject = createMockProject({
      status: 'stopped',
      deployment_status: 'cancelled',
    })
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: cancelledProject })

    render(
      <MemoryRouter
        initialEntries={[{
          pathname: '/projects/proj-123',
          state: { projectCreation: { projectName: 'billing-service' } },
        }]}
      >
        <Routes>
          <Route path="/projects/:uid" element={<ProjectDetail />} />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      expect(screen.getByText('Project created, deployment needs attention')).toBeInTheDocument()
    })

    expect(screen.getByText(/The project is saved, but the first deployment failed/i)).toBeInTheDocument()
    expect(screen.queryByText(/The first deployment is still running/i)).not.toBeInTheDocument()

    // Supports dismissal
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(screen.queryByText('Project created, deployment needs attention')).not.toBeInTheDocument()
  })

  it('renders attention/failed banner when project creation deployment status is rollback', async () => {
    const rollbackProject = createMockProject({
      status: 'stopped',
      deployment_status: 'rollback',
    })
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: rollbackProject })

    render(
      <MemoryRouter
        initialEntries={[{
          pathname: '/projects/proj-123',
          state: { projectCreation: { projectName: 'billing-service' } },
        }]}
      >
        <Routes>
          <Route path="/projects/:uid" element={<ProjectDetail />} />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      expect(screen.getByText('Project created, deployment needs attention')).toBeInTheDocument()
    })

    expect(screen.getByText(/The project is saved, but the first deployment failed/i)).toBeInTheDocument()
    expect(screen.queryByText(/The first deployment is still running/i)).not.toBeInTheDocument()
  })

  it('renders ready banner when project creation completes successfully', async () => {
    const readyProject = createMockProject({
      status: 'running',
      deployment_status: 'completed',
    })
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: readyProject })

    render(
      <MemoryRouter
        initialEntries={[{
          pathname: '/projects/proj-123',
          state: { projectCreation: { projectName: 'billing-service' } },
        }]}
      >
        <Routes>
          <Route path="/projects/:uid" element={<ProjectDetail />} />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      expect(screen.getByText('Project is ready')).toBeInTheDocument()
    })

    expect(screen.getByText(/The first deployment completed and your application is online/i)).toBeInTheDocument()
  })

  it('renders deploying banner while initial deployment is still active', async () => {
    const deployingProject = createMockProject({
      status: 'building',
      deployment_status: 'building',
    })
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: deployingProject })

    render(
      <MemoryRouter
        initialEntries={[{
          pathname: '/projects/proj-123',
          state: { projectCreation: { projectName: 'billing-service' } },
        }]}
      >
        <Routes>
          <Route path="/projects/:uid" element={<ProjectDetail />} />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      expect(screen.getByText('Preparing billing-service')).toBeInTheDocument()
    })

    expect(screen.getByText('Initial deployment')).toBeInTheDocument()
    expect(screen.getByText('Application ready')).toBeInTheDocument()
    expect(screen.queryByText(/The first deployment is still running/i)).not.toBeInTheDocument()
  })
})
