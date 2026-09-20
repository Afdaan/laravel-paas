import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Project } from '@/types'
import { projectsAPI } from '@/services/api'
import ProjectDeployment from './ProjectDeployment'

vi.mock('@/lib/usePolling', () => ({ usePolling: vi.fn() }))
vi.mock('@/services/api', () => ({
  projectsAPI: {
    get: vi.fn(),
    redeploy: vi.fn(),
  },
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

const messages: Record<string, string> = {
  'common.retry': 'Retry',
  'projectDetail.provisioning.eyebrow': 'Deployment status',
  'projectDetail.provisioning.loading': 'Loading deployment status...',
  'projectDetail.provisioning.loadFailed': 'Deployment status unavailable',
  'projectDetail.provisioning.loadFailedDesc': 'Could not load status.',
  'projectDetail.provisioning.backToProjects': 'Back to projects',
  'projectDetail.provisioning.statusDeploying': 'In progress',
  'projectDetail.provisioning.statusReady': 'Ready',
  'projectDetail.provisioning.statusFailed': 'Needs attention',
  'projectDetail.provisioning.deployingTitle': 'Preparing {{name}}',
  'projectDetail.provisioning.deployingDesc': 'Building application.',
  'projectDetail.provisioning.readyTitle': '{{name}} is ready',
  'projectDetail.provisioning.readyDesc': 'Deployment completed.',
  'projectDetail.provisioning.failedTitle': '{{name}} could not be deployed',
  'projectDetail.provisioning.failedDesc': 'Review Build Logs.',
  'projectDetail.provisioning.progress': 'Deployment progress',
  'projectDetail.provisioning.currentActivity': 'Current activity',
  'projectDetail.provisioning.waitingMessage': 'Waiting for update...',
  'projectDetail.provisioning.openProject': 'Open project',
  'projectDetail.provisioning.viewBuildLogs': 'View Build Logs',
  'projectDetail.provisioning.retryDeployment': 'Retry deployment',
  'projectDetail.provisioning.retryStarted': 'Deployment retry queued',
  'projectDetail.provisioning.retryFailed': 'Failed to queue retry',
  'projectDetail.provisioning.refresh': 'Refresh status',
  'projectDetail.provisioning.stepCreated': 'Project created',
  'projectDetail.provisioning.stepDeployment': 'Initial deployment',
  'projectDetail.provisioning.stepReady': 'Application ready',
  'projectDetail.provisioning.deploymentId': 'Deployment ID',
  'projectDetail.provisioning.pendingId': 'Pending assignment',
  'projectDetail.provisioning.startedAt': 'Started',
}

vi.mock('@/lib/useTranslation', () => ({
  default: () => ({
    language: 'en',
    t: (key: string, data?: Record<string, unknown>) => {
      let message = messages[key] || key
      Object.entries(data || {}).forEach(([name, value]) => {
        message = message.replace(`{{${name}}}`, String(value))
      })
      return message
    },
  }),
}))

function createProject(overrides: Partial<Project> = {}): Project {
  return {
    id: 1,
    uid: 'proj-123',
    user_id: 1,
    name: 'billing-service',
    repository_url: 'https://github.com/example/billing-service',
    branch: 'main',
    php_version: '8.2',
    port: null,
    db_name: 'billing_db',
    status: 'building',
    deployment_status: 'building',
    deployment_job_id: 'job-123',
    deployment_message: 'Building application image',
    deployment_progress: 42,
    created_at: '2026-09-20T10:00:00Z',
    ...overrides,
  }
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/projects/proj-123/deployments/current']}>
      <Routes>
        <Route path="/projects/:uid/deployments/current" element={<ProjectDeployment />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('ProjectDeployment', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('reconstructs active provisioning from backend state without navigation state', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: createProject() })

    renderPage()

    expect(await screen.findByText('Preparing billing-service')).toBeInTheDocument()
    expect(screen.getByText('Building application image')).toBeInTheDocument()
    expect(screen.getByText('42%')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'View Build Logs' })).toHaveAttribute('href', '/projects/proj-123?tab=build')
  })

  it('keeps successful terminal state visible until user opens project', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({ status: 'running', deployment_status: 'completed', deployment_progress: 100 }),
    })

    renderPage()

    expect(await screen.findByText('billing-service is ready')).toBeInTheDocument()
    expect(screen.getByText('100%')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Open project' })).toHaveAttribute('href', '/projects/proj-123')
  })

  it('shows failure actions and queues a retry', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({
        data: createProject({
          status: 'failed',
          deployment_status: 'failed',
          deployment_message: 'Initial deployment could not be queued. Retry deployment.',
        }),
      })
      .mockResolvedValueOnce({
        data: createProject({
          status: 'failed',
          deployment_status: 'queued',
          deployment_message: 'Redeployment requested by user',
          deployment_progress: 0,
        }),
      })
    ;(projectsAPI.redeploy as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { deployment_status: 'failed', job_id: 'job-retry' },
    })

    renderPage()

    expect(await screen.findByText('billing-service could not be deployed')).toBeInTheDocument()
    expect(screen.getByText('Initial deployment could not be queued. Retry deployment.')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Retry deployment' }))

    await waitFor(() => {
      expect(projectsAPI.redeploy).toHaveBeenCalledWith('proj-123')
      expect(screen.getByText('Preparing billing-service')).toBeInTheDocument()
    })
  })
})
