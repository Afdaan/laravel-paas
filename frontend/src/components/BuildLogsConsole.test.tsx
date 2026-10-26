import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import BuildLogsConsole from './BuildLogsConsole'
import { projectsAPI } from '../services/api'

vi.mock('../services/api', () => ({
  getCSRFToken: vi.fn(),
  projectsAPI: {
    buildLogs: vi.fn(),
    getDeploymentEvents: vi.fn(),
  },
}))

vi.mock('@/lib/useTranslation', () => ({
  default: () => ({ t: (key: string) => key }),
}))

describe('BuildLogsConsole presentation', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.mocked(projectsAPI.getDeploymentEvents).mockResolvedValue({ data: [] } as never)
  })

  afterEach(cleanup)

  it.each([
    '>> [FAILED] DEPLOYMENT ERROR: [BUILD_FAILED] SyntaxError: Unexpected token {',
    '[BUILD SUMMARY] Build failed in 16.4s',
    'npm ERR! code ELIFECYCLE',
    'npm error code 1',
    'SyntaxError: Unexpected token {',
  ])('renders failure before stage styling: %s', async (line) => {
    vi.mocked(projectsAPI.buildLogs).mockResolvedValue({ data: { logs: line } } as never)
    render(<BuildLogsConsole projectId="test-project" status="failed" />)
    expect(await screen.findByText(line)).toHaveClass('text-rose-400')
  })

  it('keeps successful builds green', async () => {
    const line = '[BUILD SUMMARY] Application built successfully in 16.4s'
    vi.mocked(projectsAPI.buildLogs).mockResolvedValue({ data: { logs: line } } as never)
    render(<BuildLogsConsole projectId="test-project" status="running" />)
    expect(await screen.findByText(line)).toHaveClass('text-emerald-400')
  })

  it('preserves code indentation and treats application output as text', async () => {
    const line = '    <script>alert("build output")</script>'
    vi.mocked(projectsAPI.buildLogs).mockResolvedValue({ data: { logs: line } } as never)
    const { container } = render(<BuildLogsConsole projectId="test-project" status="failed" />)
    const diagnostic = await screen.findByText(line.trim())
    expect(diagnostic.textContent).toBe(line)
    expect(diagnostic.parentElement).toHaveClass('min-w-0', 'whitespace-pre-wrap', '[overflow-wrap:anywhere]')
    expect(container.querySelector('script')).toBeNull()
  })
})
