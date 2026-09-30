import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

import type { Project } from '@/types'
import { OverviewTab } from './OverviewTab'

vi.mock('@/lib/useTranslation', () => ({
  default: () => ({
    language: 'en',
    t: (key: string, data?: Record<string, unknown>) => {
      const messages: Record<string, string> = {
        'projectDetail.overview.healthNotice': 'Runtime Health Notice',
        'projectDetail.overview.healthNoticeAt': 'Detected {{time}}',
        'projectDetail.overview.healthNoticeCleared': 'This notice clears on the next deployment.',
        'projectDetail.overview.deployError': 'Deployment Error',
      }
      let message = messages[key] || key
      Object.entries(data || {}).forEach(([name, value]) => {
        message = message.replace(`{{${name}}}`, String(value))
      })
      return message
    },
  }),
}))

const OOM_NOTICE =
  'Your application was terminated by the system because it exceeded its RAM limit.'

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
    status: 'running',
    deployment_status: 'completed',
    created_at: '2026-09-20T10:00:00Z',
    ...overrides,
  } as Project
}

function renderTab(project: Project) {
  return render(
    <MemoryRouter>
      <OverviewTab
        project={project}
        isDetectedFrameworkCandidate={false}
        projectUrl="https://billing.runara.app"
        isLaravelProject
        activeCommit={null}
        onTabChange={() => {}}
      />
    </MemoryRouter>,
  )
}

describe('OverviewTab runtime health notice', () => {
  it('surfaces an OOM notice on a project whose deployment succeeded', () => {
    renderTab(createProject({
      health_notice: OOM_NOTICE,
      health_notice_at: '2026-09-23T14:05:00Z',
    }))

    expect(screen.getByText('Runtime Health Notice')).toBeInTheDocument()
    expect(screen.getByText(OOM_NOTICE)).toBeInTheDocument()

    // The project is healthy from the deployment's point of view; the notice
    // must not borrow the deployment-failure surface.
    expect(screen.queryByText('Deployment Error')).not.toBeInTheDocument()
  })

  it('states when the notice goes away', () => {
    renderTab(createProject({ health_notice: OOM_NOTICE }))

    expect(
      screen.getByText('This notice clears on the next deployment.'),
    ).toBeInTheDocument()
  })

  it('shows when the notice was raised', () => {
    renderTab(createProject({
      health_notice: OOM_NOTICE,
      health_notice_at: '2026-09-23T14:05:00Z',
    }))

    expect(screen.getByText(/^Detected /)).toBeInTheDocument()
  })

  it('omits the timestamp line when the backend sent no time', () => {
    renderTab(createProject({ health_notice: OOM_NOTICE }))

    expect(screen.queryByText(/^Detected /)).not.toBeInTheDocument()
  })

  it('renders nothing when there is no notice', () => {
    renderTab(createProject())

    expect(screen.queryByText('Runtime Health Notice')).not.toBeInTheDocument()
  })

  it('keeps the deployment-failure surface for error_log', () => {
    renderTab(createProject({
      status: 'failed',
      deployment_status: 'failed',
      error_log: 'composer install exited with code 2',
    }))

    expect(screen.getByText('Deployment Error')).toBeInTheDocument()
    expect(screen.getByText('composer install exited with code 2')).toBeInTheDocument()
    expect(screen.queryByText('Runtime Health Notice')).not.toBeInTheDocument()
  })
})
