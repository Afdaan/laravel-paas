import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Project } from '@/types'
import { projectsAPI } from '@/services/api'
import { usePolling } from '@/lib/usePolling'
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
  'projectDetail.provisioning.openFullLogs': 'Open full log',
  'projectDetail.provisioning.buildLogsLoading': 'Loading build logs...',
  'projectDetail.provisioning.buildLogsUnavailable': 'Unable to load build logs.',
  'projectDetail.provisioning.buildLogsEmpty': 'No build logs available yet.',
  'projectDetail.provisioning.step_build': 'Build',
  'projectDetail.provisioning.openSite': 'Open site',
  'projectDetail.provisioning.totalElapsed': 'Total elapsed',
  'projectDetail.provisioning.noSignal': 'Awaiting deployment',
  'projectDetail.provisioning.updateDelayed': 'Deployment updates delayed',
  'projectDetail.provisioning.heartbeatLost': 'Last worker update {{duration}} ago. Deployment may still be running.',
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
    database_name: 'billing_db',
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

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it.each([
    ['queued', 'building', '5m 00s'],
    ['preparing', 'building', '5m 00s'],
    ['building', 'building', '5m 00s'],
    ['cleanup', 'building', '5m 00s'],
    ['completed', 'running', '4m 30s'],
    ['failed', 'failed', '4m 30s'],
    ['cancelled', 'failed', '4m 30s'],
    ['rollback', 'failed', '4m 30s'],
  ] as const)('includes queue time in total elapsed for %s', async (deploymentStatus, status, duration) => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-10-01T10:05:00Z'))
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      headers: { date: 'Thu, 01 Oct 2026 10:05:00 GMT' },
      data: createProject({
        server_time: '2026-10-01T10:05:00Z',
        status,
        deployment_status: deploymentStatus,
        deployment_enqueued_at: '2026-10-01T10:00:00Z',
        deployment_started_at: '2026-10-01T10:04:00Z',
        deployment_finished_at: status !== 'building' ? '2026-10-01T10:04:30Z' : undefined,
      }),
    })

    renderPage()

    expect((await screen.findByText('Total elapsed')).parentElement).toHaveTextContent(`Total elapsed ${duration}`)
  })

  it('keeps total elapsed continuous despite a four-minute Date header offset', async () => {
    const project = createProject({ deployment_enqueued_at: '2026-10-01T10:00:00Z' })
    ;(projectsAPI.get as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({
        headers: { date: 'Thu, 01 Oct 2026 10:04:05 GMT' },
        data: { ...project, server_time: '2026-10-01T10:00:05Z', deployment_status: 'queued' },
      })
      .mockResolvedValueOnce({
        headers: { date: 'Thu, 01 Oct 2026 10:04:36 GMT' },
        data: { ...project, server_time: '2026-10-01T10:00:36Z', deployment_started_at: '2026-10-01T10:00:08Z' },
      })
      .mockResolvedValueOnce({
        headers: { date: 'Thu, 01 Oct 2026 10:05:40 GMT' },
        data: { ...project, server_time: '2026-10-01T10:01:40Z', status: 'running', deployment_status: 'completed', deployment_finished_at: '2026-10-01T10:01:08Z' },
      })

    renderPage()

    expect(await screen.findByText('5s')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(await screen.findByText('36s')).toBeInTheDocument()
    expect(screen.queryByText('4m 36s')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(await screen.findByText('1m 08s')).toBeInTheDocument()
    expect(screen.getByText('billing-service is live')).toBeInTheDocument()
  })

  it('keeps queued jobs out of worker heartbeat warnings and animates only the current step', async () => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-10-01T10:05:00Z'))
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({
        deployment_status: 'queued',
        deployment_progress: 0,
        deployment_enqueued_at: '2026-10-01T10:00:00Z',
        deployment_heartbeat_at: '2026-10-01T10:00:00Z',
      }),
    })

    const { container } = renderPage()

    await screen.findByText('Awaiting deployment')
    expect(container.querySelector('.deploy-arc-stalled')).not.toBeInTheDocument()
    expect(container.querySelectorAll('.deploy-arc')).toHaveLength(1)
    expect(screen.queryByText('Deployment updates delayed')).not.toBeInTheDocument()
  })

  it('keeps job elapsed time when a worker stalls', async () => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-10-01T10:05:00Z'))
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      headers: { date: 'Thu, 01 Oct 2026 10:05:00 GMT' },
      data: createProject({
        server_time: '2026-10-01T10:05:00Z',
        deployment_enqueued_at: '2026-10-01T10:00:00Z',
        deployment_started_at: '2026-10-01T10:00:00Z',
        deployment_heartbeat_at: '2026-10-01T10:02:00Z',
      }),
    })

    const { container } = renderPage()

    const warning = await screen.findByRole('button', { name: 'Deployment updates delayed' })
    expect(screen.getByText('Total elapsed').parentElement).toHaveTextContent('Total elapsed 5m 00s')
    expect(container.querySelector('[data-slot="badge"]')).toHaveTextContent('building')
    expect(container.querySelector('[data-slot="badge"]')).not.toHaveTextContent('5m 00s')
    expect(container.querySelector('[data-slot="badge"]')).not.toHaveClass('text-amber-600')
    expect(container.querySelector('.deploy-arc-stalled')).not.toBeInTheDocument()
    expect(container.querySelectorAll('.deploy-arc')).toHaveLength(1)
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
    fireEvent.keyDown(document, { key: 'Tab' })
    act(() => warning.focus())
    expect(await screen.findByRole('tooltip')).toHaveTextContent('Last worker update 3m 00s ago. Deployment may still be running.')
    expect(warning).toHaveAccessibleDescription('Last worker update 3m 00s ago. Deployment may still be running.')
    fireEvent.keyDown(warning, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument())
  })

  it('stops animations after failure', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({ status: 'failed', deployment_status: 'failed' }),
    })

    const { container } = renderPage()

    await screen.findByText('billing-service failed')
    expect(container.querySelector('.deploy-arc')).not.toBeInTheDocument()
    expect(screen.queryByText('Deployment updates delayed')).not.toBeInTheDocument()
  })

  it('clears old job timing while retry metadata is loading', async () => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-10-01T10:05:00Z'))
    let resolveProject: (response: { data: Project }) => void = () => {}
    const freshResponse = new Promise<{ data: Project }>(resolve => { resolveProject = resolve })
    ;(projectsAPI.get as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({
        headers: { date: 'Thu, 01 Oct 2026 10:05:00 GMT' },
        data: createProject({
          status: 'failed',
          deployment_status: 'failed',
          deployment_enqueued_at: '2026-10-01T09:00:00Z',
          deployment_started_at: '2026-10-01T09:01:00Z',
          deployment_finished_at: '2026-10-01T09:02:00Z',
          deployment_heartbeat_at: '2026-10-01T09:02:00Z',
        }),
      })
      .mockReturnValueOnce(freshResponse)
    ;(projectsAPI.redeploy as ReturnType<typeof vi.fn>).mockResolvedValue({ data: { job_id: 'job-retry' } })

    const { container } = renderPage()

    await screen.findByText('billing-service failed')
    fireEvent.click(screen.getByRole('button', { name: /Retry deployment/i }))
    await screen.findByText('Awaiting deployment')
    expect(screen.getByText('Total elapsed').parentElement).toHaveTextContent('Total elapsed —')
    expect(container.querySelector('.deploy-arc-stalled')).not.toBeInTheDocument()

    resolveProject({ data: createProject({
      server_time: '2026-10-01T10:05:00Z',
      deployment_job_id: 'job-retry',
      deployment_status: 'preparing',
      deployment_enqueued_at: '2026-10-01T10:04:30Z',
      deployment_started_at: '2026-10-01T10:04:50Z',
      deployment_heartbeat_at: '2026-10-01T10:05:00Z',
    }) })
    expect(await screen.findByText('30s')).toBeInTheDocument()
  })

  it.each([-240_000, 0, 240_000])('ignores browser clock skew of %s milliseconds', async browserOffset => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-10-01T10:00:10Z') + browserOffset)
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      headers: { date: 'Thu, 01 Oct 2026 10:04:10 GMT' },
      data: createProject({
        server_time: '2026-10-01T10:00:10Z',
        deployment_enqueued_at: '2026-10-01T10:00:05Z',
        deployment_started_at: '2026-10-01T10:00:05Z',
        deployment_heartbeat_at: '2026-10-01T10:00:05Z',
      }),
    })

    const { container } = renderPage()

    await screen.findByText('Deploying billing-service')
    expect(container.querySelector('[data-slot="badge"]')).toHaveTextContent('building')
    expect(screen.getByText('Total elapsed').parentElement).toHaveTextContent('Total elapsed 5s')
    expect(container.querySelector('[data-slot="badge"]')).not.toHaveTextContent('5s')
    expect(container.querySelector('.deploy-arc-stalled')).not.toBeInTheDocument()
    expect(container.querySelectorAll('.deploy-arc')).toHaveLength(1)
  })

  it.each([undefined, 'invalid'])('does not trust Date when server_time is %s', async serverTime => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-10-01T10:04:10Z'))
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      headers: { date: 'Thu, 01 Oct 2026 10:04:10 GMT' },
      data: createProject({
        server_time: serverTime,
        deployment_enqueued_at: '2026-10-01T10:00:05Z',
        deployment_started_at: '2026-10-01T10:00:05Z',
        deployment_heartbeat_at: '2026-10-01T10:00:05Z',
      }),
    })

    const { container } = renderPage()

    await screen.findByText('Deploying billing-service')
    expect(container.querySelector('[data-slot="badge"]')).not.toHaveTextContent('4m')
    expect(screen.getByText('Total elapsed').parentElement).toHaveTextContent('Total elapsed —')
    expect(container.querySelector('.deploy-arc-stalled')).not.toBeInTheDocument()
  })

  it('clears a previous clock when the API stops providing valid server_time', async () => {
    const project = createProject({ deployment_enqueued_at: '2026-10-01T10:00:00Z' })
    ;(projectsAPI.get as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({ data: { ...project, server_time: '2026-10-01T10:00:36Z' } })
      .mockResolvedValueOnce({
        headers: { date: 'Thu, 01 Oct 2026 10:04:36 GMT' },
        data: { ...project, server_time: 'invalid' },
      })

    renderPage()

    expect(await screen.findByText('36s')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(screen.getByText('Total elapsed').parentElement).toHaveTextContent('Total elapsed —'))
  })

  it('does not add request processing time to the response clock', async () => {
    let observedAt = 0
    vi.spyOn(performance, 'now').mockImplementation(() => observedAt)
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockImplementationOnce(async () => {
      observedAt = 240_000
      return { data: createProject({ server_time: '2026-10-01T10:00:36Z', deployment_enqueued_at: '2026-10-01T10:00:00Z' }) }
    })

    renderPage()

    expect((await screen.findByText('Total elapsed')).parentElement).toHaveTextContent('Total elapsed 36s')
  })

  it('does not substitute worker start time for a missing enqueue timestamp', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      headers: { date: 'Thu, 01 Oct 2026 10:05:00 GMT' },
      data: createProject({ server_time: '2026-10-01T10:05:00Z', deployment_started_at: '2026-10-01T10:04:00Z' }),
    })

    renderPage()

    expect((await screen.findByText('Total elapsed')).parentElement).toHaveTextContent('Total elapsed —')
  })

  it('renders the GitHub icon and commit link from the backend repository field', async () => {
    const project = createProject({
      github_url: 'https://github.com/example/billing-service.git',
      last_commit_hash: 'cd63ee8abcdef',
    })
    Reflect.deleteProperty(project, 'repository_url')
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: project })

    renderPage()

    expect(await screen.findByRole('img', { name: 'GitHub' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'cd63ee8' })).toHaveAttribute(
      'href', 'https://github.com/example/billing-service/commit/cd63ee8abcdef',
    )
  })

  it('reconstructs active provisioning from backend state without navigation state', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: createProject() })

    renderPage()

    expect(await screen.findByText('Deploying billing-service')).toBeInTheDocument()
    expect(screen.getAllByText('Building application image').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('42%')).toBeInTheDocument()
    expect(screen.getByText('Build').closest('summary')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /Build Logs/i })).not.toBeInTheDocument()
  })

  it('keeps successful terminal state visible until user opens project', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({ status: 'running', deployment_status: 'completed', deployment_progress: 100 }),
    })

    const { container } = renderPage()

    expect(await screen.findByText('billing-service is live')).toBeInTheDocument()
    expect(container.querySelector('[data-slot="badge"]')).not.toBeInTheDocument()
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
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>).mockReset().mockResolvedValue({
      data: { logs: '', placeholder: true },
    })
  })

  async function openBuildLogs() {
    const summary = (await screen.findByText('Build')).closest('summary')!
    fireEvent.click(summary)
    await screen.findByRole('region', { name: 'Build output' })
    return summary
  }

  it('loads logs inside the Build step only while expanded', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: createProject() })
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { logs: '#12 3.004 Writing lock file', placeholder: false },
    })

    renderPage()

    await screen.findByText('Deploying billing-service')
    expect(projectsAPI.buildLogs).not.toHaveBeenCalled()
    const summary = await openBuildLogs()
    expect(await screen.findByText('#12 3.004 Writing lock file')).toBeInTheDocument()
    expect(projectsAPI.buildLogs).toHaveBeenCalledWith('proj-123', 200)
    expect(within(summary.closest('li')!).getByRole('region', { name: 'Build output' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Open full log' })).toHaveAttribute('href', '/projects/proj-123?tab=build')
    expect(vi.mocked(usePolling).mock.calls.some(([, delay]) => delay === 3_000)).toBe(true)

    fireEvent.click(summary)
    await waitFor(() => expect(screen.queryByRole('region', { name: 'Build output' })).not.toBeInTheDocument())
    await openBuildLogs()
    await waitFor(() => expect(projectsAPI.buildLogs).toHaveBeenCalledTimes(2))
  })

  it('shows an empty state instead of API placeholder text', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: createProject() })
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { logs: 'Initializing build environment...', placeholder: true },
    })

    renderPage()

    await openBuildLogs()
    expect(await screen.findByText('No build logs available yet.')).toBeInTheDocument()
    expect(screen.queryByText('Initializing build environment...')).not.toBeInTheDocument()
  })

  it('shows loading until the first log response arrives', async () => {
    let releaseBuildLogs: (value: unknown) => void = () => {}
    const pending = new Promise(resolve => { releaseBuildLogs = resolve })
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: createProject() })
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>).mockReturnValue(pending)

    renderPage()
    await openBuildLogs()
    expect(screen.getByText('Loading build logs...')).toBeInTheDocument()
    await act(async () => { releaseBuildLogs({ data: { logs: 'Build finished' } }) })
    expect(await screen.findByText('Build finished')).toBeInTheDocument()
  })

  it('can load completed deployment logs without polling', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({ status: 'running', deployment_status: 'completed' }),
    })
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>).mockResolvedValue({ data: { logs: 'Completed build output' } })

    renderPage()
    await openBuildLogs()
    expect(await screen.findByText('Completed build output')).toBeInTheDocument()
    expect(vi.mocked(usePolling).mock.calls.some(([, delay]) => delay === 3_000)).toBe(false)
  })

  it('refreshes the final snapshot and rejects a late response from the previous stage', async () => {
    let releaseBuildLogs: (value: unknown) => void = () => {}
    const pending = new Promise(resolve => { releaseBuildLogs = resolve })

    ;(projectsAPI.get as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({ data: createProject() })
      .mockResolvedValueOnce({
        data: createProject({ deployment_status: 'healthchecking', deployment_progress: 65 }),
      })
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>)
      .mockReturnValueOnce(pending)
      .mockResolvedValueOnce({ data: { logs: 'Final build output', job_id: 'job-123' } })

    renderPage()

    await openBuildLogs()

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(await screen.findByText('Final build output')).toBeInTheDocument()
    await act(async () => { releaseBuildLogs({ data: { logs: 'stale build output', placeholder: false } }) })
    expect(screen.queryByText('stale build output')).not.toBeInTheDocument()
    expect(screen.getByText('Final build output')).toBeInTheDocument()
  })

  it('rejects logs from a different deployment job', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: createProject() })
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { logs: 'Another job output', job_id: 'job-other' },
    })

    renderPage()
    await openBuildLogs()
    expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load build logs.')
    expect(screen.queryByText('Another job output')).not.toBeInTheDocument()
  })

  it('resets logs and ignores pending output when a new job replaces the current one', async () => {
    let releaseBuildLogs: (value: unknown) => void = () => {}
    const pending = new Promise(resolve => { releaseBuildLogs = resolve })
    ;(projectsAPI.get as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({ data: createProject() })
      .mockResolvedValueOnce({ data: createProject({ deployment_job_id: 'job-new' }) })
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>)
      .mockReturnValueOnce(pending)
      .mockResolvedValueOnce({ data: { logs: 'New job output', job_id: 'job-new' } })

    renderPage()
    await openBuildLogs()
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(await screen.findByText('New job output')).toBeInTheDocument()
    await act(async () => { releaseBuildLogs({ data: { logs: 'Old job output', job_id: 'job-123' } }) })
    expect(screen.queryByText('Old job output')).not.toBeInTheDocument()
    expect(screen.getByText('New job output')).toBeInTheDocument()
  })

  it('shows a retry action after a failed log request', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: createProject() })
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>)
      .mockRejectedValueOnce(new Error('network failure'))
      .mockResolvedValueOnce({ data: { logs: 'Recovered build output' } })

    renderPage()
    await openBuildLogs()
    expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load build logs.')
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Recovered build output')).toBeInTheDocument()
  })

  it('renders log markup as plain text', async () => {
    const output = '<img src=x onerror=alert(1)>'
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({ data: createProject() })
    ;(projectsAPI.buildLogs as ReturnType<typeof vi.fn>).mockResolvedValue({ data: { logs: output } })

    renderPage()
    await openBuildLogs()
    expect(await screen.findByText(output)).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Build output' }).querySelector('img')).toBeNull()
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

  it('fills the source, runtime, and database cells from what the worker detected', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({
        framework: 'Laravel',
        laravel_version: '11',
        php_version: '8.3',
        internal_port: '8000',
        last_commit_hash: 'cd63ee8aa',
        database_option: 'new',
        database_instance: { engine: 'postgresql', version: '16', name: 'billing_db' } as Project['database_instance'],
      }),
    })
    ;(projectsAPI.getDeploymentEvents as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: [{ id: 1, project_id: 1, job_id: 'job-123', sequence_number: 1, event_type: 'building_image', payload: 'Commit cd63ee8: feat: add heart reaction', created_at: '' }],
    })

    renderPage()

    expect(await screen.findByText('feat: add heart reaction')).toBeInTheDocument()
    expect(screen.getByText('Laravel 11')).toBeInTheDocument()
    expect(screen.getByText('PHP 8.3 · projectDetail.provisioning.runtimePort')).toBeInTheDocument()
    expect(screen.getByText('billing_db')).toBeInTheDocument()
    expect(screen.getByText('PostgreSQL 16')).toBeInTheDocument()
  })

  it('says a project has no database instead of waiting for one', async () => {
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({ database_option: 'none', database_name: '' }),
    })

    renderPage()

    expect(await screen.findByText('projectDetail.provisioning.noDatabase')).toBeInTheDocument()
  })

  it('does not promise pending metadata after a failure', async () => {
    // Failing this early means runtime detection and the database never ran.
    ;(projectsAPI.get as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: createProject({
        status: 'failed',
        deployment_status: 'failed',
        php_version: '',
        database_name: '',
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
      data: createProject({ php_version: '', database_name: '' }),
    })

    renderPage()
    await screen.findByText('Deploying billing-service')

    expect(screen.getAllByText('Pending').length).toBeGreaterThanOrEqual(2)
    expect(screen.queryByText('Not detected')).not.toBeInTheDocument()
  })
})
