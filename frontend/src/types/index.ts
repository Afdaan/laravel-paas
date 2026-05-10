export interface User {
  id: number;
  name: string;
  email: string;
  role: 'student' | 'admin' | 'superadmin';
  password?: string;
  last_login?: string;
  last_activity?: string;
  last_ip?: string;
  last_location?: string;
  created_at?: string;
}

export interface Project {
  id: number;
  uid: string;
  user_id: number;
  name: string;
  repository_url: string;
  branch: string;
  php_version: string;
  port: number;
  db_name: string;
  status: 'running' | 'stopped' | 'error' | 'deploying' | 'building' | 'failed' | 'pending' | 'queued' | 'restarting';
  subdomain?: string;
  url?: string;
  user?: User;
  laravel_version?: string;
  github_url?: string;
  error_log?: string;
  queue_enabled?: boolean;
  is_manual_version?: boolean;
  container_id?: string;
  database_name?: string;
  base_directory?: string;
  worker_command?: string;
  worker_container_id?: string;
  build_command?: string;
  start_command?: string;
  node_version?: string;
  framework?: string;
  language_version?: string;
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
