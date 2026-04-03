export interface User {
  id: number;
  name: string;
  email: string;
  role: 'student' | 'admin' | 'superadmin';
  created_at?: string;
}

export interface Project {
  id: number;
  user_id: number;
  name: string;
  repository_url: string;
  branch: string;
  php_version: string;
  port: number;
  db_name: string;
  status: 'running' | 'stopped' | 'error' | 'deploying';
  subdomain?: string;
  url?: string;
  user?: User;
  laravel_version?: string;
  github_url?: string;
  error_log?: string;
  queue_enabled?: boolean;
  is_manual_version?: boolean;
}

export interface ProjectStats {
  cpu_percent: number;
  memory_mb: number;
  memory_max_mb: number;
}

export interface AuthResponse {
  token: string;
  user: User;
}

export interface SystemStats {
  projects_count: number;
  users_count: number;
  active_containers: number;
  total_databases: number;
  memory_usage?: string;
  cpu_usage?: string;
}
