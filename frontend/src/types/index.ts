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
  status: 'deploying' | 'running' | 'failed' | 'stopped';
  container_id?: string;
  created_at: string;
  url?: string;
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
