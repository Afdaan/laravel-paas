import { memo } from 'react'
import BuildLogsConsole from '@/components/BuildLogsConsole'
import { Project } from '@/types'

interface BuildTabProps {
  project: Project
  onDeploymentEvent: () => void
}

export const BuildTab = memo(function BuildTab({ project, onDeploymentEvent }: BuildTabProps) {
  if (!project) return null

  return (
    <BuildLogsConsole
      key={`${project.uid}:${project.deployment_job_id || 'no-job'}`}
      projectId={project.uid}
      status={project.status}
      project={project}
      onDeploymentEvent={onDeploymentEvent}
    />
  )
})
