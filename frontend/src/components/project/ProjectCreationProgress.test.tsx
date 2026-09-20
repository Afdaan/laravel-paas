import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import {
  ProjectCreationBanner,
  ProjectCreationLoading,
} from './ProjectCreationProgress'
import {
  getProjectCreationPhase,
  isFailedDeploymentStatus,
  isTerminalDeploymentStatus,
  TERMINAL_DEPLOYMENT_STATUSES,
  FAILED_DEPLOYMENT_STATUSES,
  getProjectCreationContext,
} from './projectCreationContext'

const messages: Record<string, string> = {
  'common.close': 'Close',
  'projectDetail.messages.creationProjectFallback': 'your project',
  'projectDetail.messages.creationCreated': 'Project created',
  'projectDetail.messages.creationPreparingTitle': 'Preparing {{name}}',
  'projectDetail.messages.creationPreparingDesc': 'Project saved and deployment starting.',
  'projectDetail.messages.creationDeployment': 'Initial deployment',
  'projectDetail.messages.creationReady': 'Application ready',
  'projectDetail.messages.creationSuccessTitle': 'Project created successfully',
  'projectDetail.messages.creationSuccessDesc': 'Deployment still running.',
  'projectDetail.messages.creationReadyTitle': 'Project is ready',
  'projectDetail.messages.creationReadyDesc': 'Application is online.',
  'projectDetail.messages.creationFailedTitle': 'Deployment needs attention',
  'projectDetail.messages.creationFailedDesc': 'Open Build Logs.',
}

vi.mock('@/lib/useTranslation', () => ({
  default: () => ({
    t: (key: string, data?: Record<string, string | number>) => {
      let value = messages[key] || key
      Object.entries(data || {}).forEach(([name, replacement]) => {
        value = value.replace(`{{${name}}}`, String(replacement))
      })
      return value
    },
  }),
}))

describe('ProjectCreationProgress', () => {
  it('shows project creation and initial deployment progress', () => {
    render(<ProjectCreationLoading projectName="billing-api" />)

    expect(screen.getByRole('status')).toHaveTextContent('Preparing billing-api')
    expect(screen.getAllByText('Project created')).toHaveLength(2)
    expect(screen.getByText('Initial deployment')).toBeInTheDocument()
    expect(screen.getByText('Application ready')).toBeInTheDocument()
  })

  it.each([
    ['ready', 'Project is ready'],
    ['failed', 'Deployment needs attention'],
  ] as const)('shows the %s state and supports dismissal', (phase, title) => {
    const onDismiss = vi.fn()
    render(<ProjectCreationBanner phase={phase} onDismiss={onDismiss} />)

    expect(screen.getByRole('status')).toHaveTextContent(title)
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onDismiss).toHaveBeenCalledOnce()
  })

  it('accepts only structured project creation navigation state', () => {
    expect(getProjectCreationContext({ projectCreation: { projectName: 'billing-api' } })).toEqual({ projectName: 'billing-api' })
    expect(getProjectCreationContext({ projectCreation: 'billing-api' })).toBeNull()
  })

  describe('terminal deployment helpers', () => {
    it('accurately identifies terminal deployment statuses', () => {
      for (const status of TERMINAL_DEPLOYMENT_STATUSES) {
        expect(isTerminalDeploymentStatus(status)).toBe(true)
      }
      expect(isTerminalDeploymentStatus('queued')).toBe(false)
      expect(isTerminalDeploymentStatus('building')).toBe(false)
      expect(isTerminalDeploymentStatus('migrating')).toBe(false)
      expect(isTerminalDeploymentStatus(null)).toBe(false)
      expect(isTerminalDeploymentStatus(undefined)).toBe(false)
    })

    it('accurately identifies failed/aborted deployment statuses', () => {
      for (const status of FAILED_DEPLOYMENT_STATUSES) {
        expect(isFailedDeploymentStatus(status)).toBe(true)
      }
      expect(isFailedDeploymentStatus('completed')).toBe(false)
      expect(isFailedDeploymentStatus('building')).toBe(false)
      expect(isFailedDeploymentStatus(null)).toBe(false)
      expect(isFailedDeploymentStatus(undefined)).toBe(false)
    })
  })

  describe('getProjectCreationPhase', () => {
    it.each([
      ['cancelled', 'failed'],
      ['rollback', 'failed'],
      ['failed', 'failed'],
    ] as const)('maps terminal non-completed deployment status %s to failed phase', (deploymentStatus, expectedPhase) => {
      const phase = getProjectCreationPhase({
        status: 'stopped',
        deployment_status: deploymentStatus,
      })
      expect(phase).toBe(expectedPhase)
    })

    it('maps project status failed to failed phase regardless of deployment status', () => {
      expect(getProjectCreationPhase({ status: 'failed', deployment_status: 'building' })).toBe('failed')
      expect(getProjectCreationPhase({ status: 'failed', deployment_status: null })).toBe('failed')
    })

    it('maps running project with completed or missing deployment status to ready phase', () => {
      expect(getProjectCreationPhase({ status: 'running', deployment_status: 'completed' })).toBe('ready')
      expect(getProjectCreationPhase({ status: 'running', deployment_status: null })).toBe('ready')
      expect(getProjectCreationPhase({ status: 'running', deployment_status: undefined })).toBe('ready')
    })

    it('maps active in-flight deployment to deploying phase', () => {
      expect(getProjectCreationPhase({ status: 'building', deployment_status: 'building' })).toBe('deploying')
      expect(getProjectCreationPhase({ status: 'queued', deployment_status: 'queued' })).toBe('deploying')
      expect(getProjectCreationPhase({ status: 'running', deployment_status: 'building' })).toBe('deploying')
    })
  })

  describe('creation banner outcomes for cancelled and rollback', () => {
    it('renders attention/failed banner when creation deployment was cancelled', () => {
      const onDismiss = vi.fn()
      const phase = getProjectCreationPhase({
        status: 'stopped',
        deployment_status: 'cancelled',
      })
      expect(phase).toBe('failed')

      render(<ProjectCreationBanner phase={phase} onDismiss={onDismiss} />)
      expect(screen.getByRole('status')).toHaveTextContent('Deployment needs attention')
      expect(screen.getByText('Open Build Logs.')).toBeInTheDocument()

      fireEvent.click(screen.getByRole('button', { name: 'Close' }))
      expect(onDismiss).toHaveBeenCalledOnce()
    })

    it('renders attention/failed banner when creation deployment rolled back', () => {
      const onDismiss = vi.fn()
      const phase = getProjectCreationPhase({
        status: 'stopped',
        deployment_status: 'rollback',
      })
      expect(phase).toBe('failed')

      render(<ProjectCreationBanner phase={phase} onDismiss={onDismiss} />)
      expect(screen.getByRole('status')).toHaveTextContent('Deployment needs attention')
      expect(screen.getByText('Open Build Logs.')).toBeInTheDocument()

      fireEvent.click(screen.getByRole('button', { name: 'Close' }))
      expect(onDismiss).toHaveBeenCalledOnce()
    })
  })
})
