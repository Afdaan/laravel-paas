import { LayoutGrid, Cpu, Terminal as TerminalIcon, Key, Database as DatabaseIcon, Scroll, Hammer, Globe, Settings } from 'lucide-react'

export type ProjectDetailTab =
  | 'project'
  | 'runtime'
  | 'console'
  | 'environment'
  | 'database'
  | 'logs'
  | 'build'
  | 'domains'
  | 'settings'

export const PROJECT_DETAIL_TABS = [
  { value: 'project', labelKey: 'projectDetail.tabs.overview', icon: LayoutGrid },
  { value: 'runtime', labelKey: 'projectDetail.tabs.runtime', icon: Cpu },
  { value: 'console', labelKey: 'projectDetail.tabs.console', icon: TerminalIcon },
  { value: 'environment', labelKey: 'projectDetail.tabs.secrets', icon: Key },
  { value: 'database', labelKey: 'projectDetail.tabs.database', icon: DatabaseIcon },
  { value: 'logs', labelKey: 'projectDetail.tabs.logs', icon: Scroll },
  { value: 'build', labelKey: 'projectDetail.tabs.build', icon: Hammer },
  { value: 'domains', labelKey: 'projectDetail.tabs.domains', icon: Globe },
  { value: 'settings', labelKey: 'projectDetail.tabs.settings', icon: Settings },
] as const

export const PROJECT_DETAIL_TAB_VALUES = PROJECT_DETAIL_TABS.map(tab => tab.value)

export function isProjectDetailTab(value: string): value is ProjectDetailTab {
  return PROJECT_DETAIL_TAB_VALUES.includes(value as ProjectDetailTab)
}
