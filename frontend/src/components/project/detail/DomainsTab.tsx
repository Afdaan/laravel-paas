import { Globe } from 'lucide-react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card'
import useTranslation from '@/lib/useTranslation'
import { Project } from '@/types'
import { CustomDomainManager } from '@/components/project/CustomDomainManager'

interface DomainsTabProps {
  project: Project
  onDomainsChanged: () => void
}

export function DomainsTab({ project, onDomainsChanged }: DomainsTabProps) {
  const { t } = useTranslation()

  if (!project) return null

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-4">
          <div className="w-10 h-10 bg-primary/10 rounded-lg flex items-center justify-center text-primary">
            <Globe className="w-5 h-5" />
          </div>
          <div>
            <CardTitle className="text-lg">{t('projectDetail.settings.customDomain') || 'Custom Domain'}</CardTitle>
            <CardDescription>{t('projectDetail.settings.customDomainDesc') || 'Manage custom domains for your project'}</CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-6">
        <CustomDomainManager 
          projectId={project.uid} 
          subdomain={project.subdomain!} 
          projectUrl={project.url} 
          onDomainsChanged={onDomainsChanged} 
        />
      </CardContent>
    </Card>
  )
}
