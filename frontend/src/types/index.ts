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

export interface CustomDomain {
  id: number;
  domain: string;
  status: 'active' | 'pending' | 'error';
  project_id: number;
  project?: Project;
  created_at: string;
  updated_at: string;
}

export interface DomainDiagnostic {
  domain: string;
  expected_type: string;
  expected_host: string;
  expected_value: string;
  current_cname: string;
  current_ips: string[];
  is_match: boolean;
  message: string;
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
  custom_domains?: CustomDomain[];
  deployment_status?: 'queued' | 'preparing' | 'cloning' | 'building' | 'provisioning' | 'starting' | 'healthchecking' | 'migrating' | 'promoting' | 'cleanup' | 'completed' | 'failed' | 'rollback' | 'cancelled';
  deployment_job_id?: string;
  rollout_container_id?: string;
  deployment_started_at?: string;
  deployment_finished_at?: string;
  deployment_heartbeat_at?: string;
  deployment_message?: string;
  deployment_progress?: number;
  created_at: string;
}

export interface DeploymentEvent {
  id: number;
  project_id: number;
  sequence_number: number;
  status: string;
  step_name: string;
  message: string;
  created_at: string;
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
