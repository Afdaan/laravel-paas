export type ProjectCreationPhase = 'deploying' | 'ready' | 'failed'

export const TERMINAL_DEPLOYMENT_STATUSES = ['completed', 'failed', 'rollback', 'cancelled'] as const
export type TerminalDeploymentStatus = typeof TERMINAL_DEPLOYMENT_STATUSES[number]

export const FAILED_DEPLOYMENT_STATUSES = ['failed', 'rollback', 'cancelled'] as const
export type FailedDeploymentStatus = typeof FAILED_DEPLOYMENT_STATUSES[number]

export function isTerminalDeploymentStatus(status?: string | null): boolean {
  if (!status) return false
  return (TERMINAL_DEPLOYMENT_STATUSES as readonly string[]).includes(status)
}

export function isFailedDeploymentStatus(status?: string | null): boolean {
  if (!status) return false
  return (FAILED_DEPLOYMENT_STATUSES as readonly string[]).includes(status)
}

export function getProjectCreationPhase(project: {
  status?: string | null
  deployment_status?: string | null
}): ProjectCreationPhase {
  if (project.deployment_status && isFailedDeploymentStatus(project.deployment_status)) {
    return 'failed'
  }

  if (project.deployment_status && !isTerminalDeploymentStatus(project.deployment_status)) {
    return 'deploying'
  }

  if (project.status === 'failed') return 'failed'

  if (
    project.status === 'running' &&
    (!project.deployment_status || project.deployment_status === 'completed')
  ) {
    return 'ready'
  }

  return 'deploying'
}
