export interface User {
  id: number;
  name: string;
  email: string;
  role: 'user' | 'admin' | 'superadmin';
  password?: string;
  last_login?: string;
  last_activity?: string;
  last_ip?: string;
  last_location?: string;
  created_at?: string;
  avatar_url?: string;
}

export interface CustomDomain {
  id: number;
  domain: string;
  status: string;
  is_primary?: boolean;
  health_status?: string;
  last_healthcheck_at?: string;
  error_code?: string;
  degraded_reason_code?: string;
  error_message?: string;
  config_hash?: string;
  last_verification_at?: string;
  last_renewal_attempt_at?: string;
  ssl_expires_at?: string;
  project_id: number;
  project?: Project;
  current_sequence?: number;
  created_at: string;
  updated_at: string;
}

export interface DomainEvent {
  id: number;
  domain_id: number;
  event_type: string;
  state_from: string;
  state_to: string;
  error_code?: string;
  message: string;
  payload?: string;
  created_at: string;
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
  port: number | null;
  db_name: string;
  status: 'running' | 'stopped' | 'error' | 'deploying' | 'building' | 'failed' | 'pending' | 'queued' | 'restarting';
  subdomain?: string;
  url?: string;
  user?: User;
  laravel_version?: string;
  github_url?: string;
  github_installation_id?: number | null;
  github_repo_owner?: string;
  github_repo_name?: string;
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
  detected_framework?: string;
  language_version?: string;
  cpu_limit?: number;
  memory_limit?: string;
  internal_port?: string;
  last_commit_hash?: string;
  custom_domains?: CustomDomain[];
  database_instance?: DatabaseInstance;
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
  job_id?: string;
  sequence_number: number;
  worker_id?: string;
  state_from?: string;
  state_to?: string;
  event_type?: string;
  payload?: string;
  duration_ms?: number;
  error?: string;
  status?: string;
  step_name?: string;
  message?: string;
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

export interface GithubAppInstallation {
  id: number;
  user_id: number;
  installation_id: number;
  account_name: string;
  avatar_url: string;
  created_at: string;
  updated_at: string;
}

export interface GithubRepository {
  id: number;
  name: string;
  full_name: string;
  html_url: string;
  description: string;
  private: boolean;
  default_branch: string;
}

export interface DatabaseInstance {
  id: number;
  uid: string;
  project_id: number | null;
  project?: Project;
  engine: 'mysql' | 'postgresql';
  version?: string;
  status: 'active' | 'suspended' | 'deleted';
  name: string;
  username: string;
  host: string;
  port: number | null;
  storage_allocation: number;
  storage_consumption: number;
  connection_count: number;
  created_at: string;
  updated_at: string;
  size?: string;
  table_count?: number;
  row_count?: number;
  database?: string;
  password?: string;
}

export interface DatabaseBackup {
  id: number;
  database_instance_id: number;
  project_id: number;
  name: string;
  path: string;
  size: string;
  status: 'pending' | 'completed' | 'failed';
  created_at: string;
}

export interface DatabaseMetrics {
  active_connections: number;
  size_kb: number;
}

export interface BillingCatalogSpec {
  id: number;
  type: 'project' | 'database';
  name: string;
  slug: string;
  cpu_millicores: number;
  memory_mb: number;
  storage_gb: number;
  monthly_credits: number;
  connection_limit?: number;
  backup_retention_days?: number;
  badge_text?: string;
}

export interface TopupPackage {
  id: number;
  credits: number;
  currency: string;
  amount_minor: number;
  sort_order: number;
}

export interface BillingOverview {
  wallet: {
    balance_credits: number;
    ledger_entries: Array<{
      type: string;
      amount_credits: number;
      balance_after: number;
      created_at: string;
    }>;
  };
  invoices: Array<{
    id: number;
    period_start: string;
    period_end: string;
    total_credits: number;
    status: string;
    due_at?: string;
    paid_at?: string;
    created_at: string;
  }>;
  topups: Array<{
    id: number;
    credits: number;
    amount_minor: number;
    currency: string;
    status: string;
    paid_at?: string;
    created_at: string;
  }>;
  resources: Array<{
    resource_id: number;
    resource_type: 'project' | 'database';
    resource_name: string;
    spec_name: string;
    monthly_credits: number;
    status: 'active' | 'payment_due' | 'suspended';
    current_period_start: string;
    next_invoice_at: string;
    auto_renew: boolean;
  }>;
  upcoming_required_credits: number;
}

export interface BillingStatus {
  resource_id: number;
  resource_type: 'project' | 'database';
  status: 'active' | 'payment_due' | 'suspended';
  oldest_due_at?: string;
  payment_due_days: number;
}
