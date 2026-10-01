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
    // Feeds per-step durations. The page renders without them, so the default
    // is an empty timeline rather than a fixture.
    getDeploymentEvents: vi.fn(() => Promise.resolve({ data: [] })),
    // Inline build output. Default to the placeholder response, which renders
    // nothing, so these cases stay about the pipeline.
    buildLogs: vi.fn(() => Promise.resolve({ data: { logs: '', placeholder: true } })),
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
  'projectDetail.provisioning.statusDeploying': 'Deploying',
  'projectDetail.provisioning.statusReady': 'Ready',
  'projectDetail.provisioning.statusFailed': 'Failed',
  'projectDetail.provisioning.deployingTitle': 'Deploying {{name}}',
  'projectDetail.provisioning.deployingDesc': 'Building application.',
  'projectDetail.provisioning.readyTitle': '{{name}} is live',
  'projectDetail.provisioning.readyDesc': 'Application is healthy.',
  'projectDetail.provisioning.failedTitle': '{{name}} failed',
  'projectDetail.provisioning.failedDesc': 'Check Build Logs.',
  'projectDetail.provisioning.progress': 'Deployment progress',
  'projectDetail.provisioning.pipelineTitle': 'Pipeline',
  'projectDetail.provisioning.currentActivity': 'Activity',
  'projectDetail.provisioning.waitingMessage': 'Waiting for update...',
  'projectDetail.provisioning.openProject': 'Open project',
  'projectDetail.provisioning.viewBuildLogs': 'Build Logs',
  'projectDetail.provisioning.retryDeployment': 'Retry deployment',
  'projectDetail.provisioning.retryStarted': 'Deployment retry queued',
  'projectDetail.provisioning.retryFailed': 'Failed to queue retry',
  'projectDetail.provisioning.refresh': 'Refresh',
  'projectDetail.provisioning.copyUrl': 'Copy URL',
  'projectDetail.provisioning.copied': 'Copied',
  'projectDetail.provisioning.metaDomain': 'Domain',
  'projectDetail.provisioning.metaSource': 'Source',
  'projectDetail.provisioning.metaTiming': 'Started',
  'projectDetail.provisioning.metaRuntime': 'Runtime',
  'projectDetail.provisioning.metaDuration': 'Started',
  'projectDetail.provisioning.stepSource': 'Source',
  'projectDetail.provisioning.stepSourceDetail': 'Checked out',
  'projectDetail.provisioning.stepBuild': 'Build',
  'projectDetail.provisioning.stepDatabase': 'Database',
  'projectDetail.provisioning.stepHealth': 'Health check',
  'projectDetail.provisioning.stepCreated': 'Project created',
  'projectDetail.provisioning.stepDeployment': 'Initial deployment',
  'projectDetail.provisioning.stepReady': 'Application ready',
  'projectDetail.provisioning.deploymentId': 'Deployment ID',
  'projectDetail.provisioning.pendingId': 'Pending assignment',
  'projectDetail.provisioning.startedAt': 'Started',
  'projectDetail.provisioning.openSettings': 'Open settings',
  'projectDetail.provisioning.notDetected': 'Not detected',
  'projectDetail.provisioning.pending': 'Pending',
  'projectDetail.provisioning.buildOutput': 'Build output',
  'projectDetail.provisioning.openSite': 'Open site',
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

    expect(await screen.findByText('Deploying billing-service')).toBeInTheDocument()
    expect(screen.getAllByText('Building application image').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('42%')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Build Logs/i })).toHaveAttribute('href', '/projects/proj-123?tab=build')
  })

  it('keeps successful terminal state visible until user opens project', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({ status: 'running', deployment_status: 'completed', deployment_progress: 100 }),
    })

    renderPage()

    expect(await screen.findByText('billing-service is live')).toBeInTheDocument()
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

    expect(await screen.findByText('billing-service failed')).toBeInTheDocument()
    expect(screen.getAllByText('Initial deployment could not be queued. Retry deployment.').length).toBeGreaterThanOrEqual(1)

    fireEvent.click(screen.getByRole('button', { name: /Retry deployment/i }))

    await waitFor(() => {
      expect(projectsAPI.redeploy).toHaveBeenCalledWith('proj-123')
      expect(screen.getByText('Deploying billing-service')).toBeInTheDocument()
    })
  })
})

describe('ProjectDeployment build output', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows build output while the build is the running stage', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: createProject() })
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { logs: '#12 3.004 Writing lock file', placeholder: false },
    })

    renderPage()

    expect(await screen.findByText('#12 3.004 Writing lock file')).toBeInTheDocument()
  })

  it('renders nothing for a placeholder response', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: createProject() })
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { logs: 'Initializing build environment...', placeholder: true },
    })

    renderPage()

    await screen.findByText('Deploying billing-service')
    expect(screen.queryByText('Initializing build environment...')).not.toBeInTheDocument()
  })

  it('discards a build-tail response that arrives after the stage advanced', async () => {
    // Held open so it can resolve after the deployment has moved past building.
    let releaseBuildLogs: (value: unknown) => void = () => {}
    const pending = new Promise(resolve => { releaseBuildLogs = resolve })

    ;(projectsAPI.get as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({ data: createProject() })
      .mockResolvedValueOnce({
        data: createProject({ deployment_status: 'healthchecking', deployment_progress: 65 }),
      })
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>).mockReturnValue(pending)

    renderPage()

    await screen.findByText('Deploying billing-service')

    // Refresh pulls the project forward to a stage where build output no longer applies.
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => {
      expect(projectsAPI.get).toHaveBeenCalledTimes(2)
    })

    releaseBuildLogs({ data: { logs: 'stale build output', placeholder: false } })

    await waitFor(() => {
      expect(screen.queryByText('stale build output')).not.toBeInTheDocument()
    })
  })
})

describe('ProjectDeployment failure remedies', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  function failWith(error: string) {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({
        status: 'failed',
        deployment_status: 'failed',
        subdomain: 'billing.runara.app',
        error_log: error,
      }),
    })
  }

  it('offers the settings tab when retrying cannot change the outcome', async () => {
    failWith('[INVALID_BASE_DIRECTORY] Configured base directory does not exist.')

    renderPage()
    await screen.findByText('billing-service failed')

    expect(screen.getByRole('link', { name: /Open settings/i }))
      .toHaveAttribute('href', '/projects/proj-123?tab=settings')
    // Retry stays reachable; it just stops being the loudest thing on screen.
    expect(screen.getByRole('button', { name: /Retry deployment/i })).toBeInTheDocument()
  })

  it('leaves retry alone for a failure it cannot route', async () => {
    failWith('Initial deployment could not be queued.')

    renderPage()
    await screen.findByText('billing-service failed')

    expect(screen.queryByRole('link', { name: /Open settings/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Retry deployment/i })).toBeInTheDocument()
  })

  it('reads the sentence without the machine code, and keeps the code for support', async () => {
    failWith('[INVALID_BASE_DIRECTORY] Configured base directory does not exist.')

    renderPage()
    await screen.findByText('billing-service failed')

    expect(screen.getByText('Configured base directory does not exist.')).toBeInTheDocument()
    expect(screen.queryByText(/\[INVALID_BASE_DIRECTORY\]/)).not.toBeInTheDocument()
    // Still on the page, demoted to the footer beside the job id.
    expect(screen.getByText('INVALID_BASE_DIRECTORY')).toBeInTheDocument()
  })

  it('names the domain without linking to it when nothing ever served', async () => {
    failWith('[BUILD_FAILED] build broke')

    renderPage()
    await screen.findByText('billing-service failed')

    expect(screen.getByText('billing.runara.app')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Open site' })).not.toBeInTheDocument()
  })

  it('links the domain once the deployment succeeded', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({
        status: 'running',
        deployment_status: 'completed',
        deployment_progress: 100,
        subdomain: 'billing.runara.app',
      }),
    })

    renderPage()
    await screen.findByText('billing-service is live')

    expect(screen.getByText('billing.runara.app')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Open site' }))
      .toHaveAttribute('href', 'https://billing.runara.app')
  })

  it('links the commit to the repository', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({
        repository_url: 'https://github.com/example/billing-service.git',
        last_commit_hash: 'a1b2c3d4e5f6',
      }),
    })

    renderPage()

    expect(await screen.findByRole('link', { name: 'a1b2c3d' }))
      .toHaveAttribute('href', 'https://github.com/example/billing-service/commit/a1b2c3d4e5f6')
  })

  it('does not promise pending metadata after a failure', async () => {
    // Failing this early means runtime detection and the database never ran.
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({
        status: 'failed',
        deployment_status: 'failed',
        php_version: '',
        db_name: '',
        error_log: '[INVALID_BASE_DIRECTORY] Configured base directory does not exist.',
      }),
    })

    renderPage()
    await screen.findByText('billing-service failed')

    // "Pending" would promise a value that is never arriving.
    expect(screen.getAllByText('Not detected').length).toBeGreaterThanOrEqual(2)
    expect(screen.queryByText('Pending')).not.toBeInTheDocument()
  })

  it('still says pending while the deployment is running', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({ php_version: '', db_name: '' }),
    })

    renderPage()
    await screen.findByText('Deploying billing-service')

    expect(screen.getAllByText('Pending').length).toBeGreaterThanOrEqual(2)
    expect(screen.queryByText('Not detected')).not.toBeInTheDocument()
  })
})
