import { describe, expect, it } from 'vitest'

import {
  FAILED_DEPLOYMENT_STATUSES,
  TERMINAL_DEPLOYMENT_STATUSES,
  getProjectCreationPhase,
  isFailedDeploymentStatus,
  isTerminalDeploymentStatus,
} from './projectCreationContext'

describe('project creation deployment state', () => {
  it('recognizes terminal and failed deployment statuses', () => {
    TERMINAL_DEPLOYMENT_STATUSES.forEach(status => expect(isTerminalDeploymentStatus(status)).toBe(true))
    FAILED_DEPLOYMENT_STATUSES.forEach(status => expect(isFailedDeploymentStatus(status)).toBe(true))
    expect(isTerminalDeploymentStatus('building')).toBe(false)
    expect(isFailedDeploymentStatus('completed')).toBe(false)
  })

  it('maps completed running projects to ready', () => {
    expect(getProjectCreationPhase({ status: 'running', deployment_status: 'completed' })).toBe('ready')
  })

  it('keeps an active retry deploying while project status is still stale', () => {
    expect(getProjectCreationPhase({ status: 'failed', deployment_status: 'queued' })).toBe('deploying')
    expect(getProjectCreationPhase({ status: 'failed', deployment_status: 'building' })).toBe('deploying')
  })

  it.each(['failed', 'rollback', 'cancelled'])('maps %s deployments to failed', deploymentStatus => {
    expect(getProjectCreationPhase({ status: 'stopped', deployment_status: deploymentStatus })).toBe('failed')
  })
})
