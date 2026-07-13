export interface RuntimeVersion {
  value: string;
  label: string;
  isLatest?: boolean;
}

export const DEFAULT_RUNTIME_VERSIONS = {
  php: '8.4',
  node: '22',
  go: '1.25',
  python: '3.13',
};

export function getDisplayedFramework(project?: { framework?: string; detected_framework?: string } | null) {
  // Keep runtime controls on the promoted framework while showing the latest source detection.
  return project?.detected_framework || project?.framework
}

export const RUNTIME_VERSIONS: Record<string, RuntimeVersion[]> = {
  php: [
    { value: '8.0', label: 'PHP 8.0' },
    { value: '8.1', label: 'PHP 8.1' },
    { value: '8.2', label: 'PHP 8.2' },
    { value: '8.3', label: 'PHP 8.3' },
    { value: '8.4', label: 'PHP 8.4', isLatest: true },
  ],
  node: [
    { value: '14', label: 'Node.js 14' },
    { value: '16', label: 'Node.js 16' },
    { value: '18', label: 'Node.js 18 (LTS)' },
    { value: '20', label: 'Node.js 20 (LTS)' },
    { value: '22', label: 'Node.js 22 (LTS)' },
    { value: '23', label: 'Node.js 23 (Current)' },
  ],
  go: [
    { value: '1.18', label: 'Go 1.18' },
    { value: '1.19', label: 'Go 1.19' },
    { value: '1.20', label: 'Go 1.20' },
    { value: '1.21', label: 'Go 1.21' },
    { value: '1.22', label: 'Go 1.22' },
    { value: '1.23', label: 'Go 1.23' },
    { value: '1.24', label: 'Go 1.24' },
    { value: '1.25', label: 'Go 1.25', isLatest: true },
  ],
  python: [
    { value: '3.8', label: 'Python 3.8' },
    { value: '3.9', label: 'Python 3.9' },
    { value: '3.10', label: 'Python 3.10' },
    { value: '3.11', label: 'Python 3.11' },
    { value: '3.12', label: 'Python 3.12' },
    { value: '3.13', label: 'Python 3.13', isLatest: true },
  ]
};
