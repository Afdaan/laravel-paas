# Laravel PaaS (Platform as a Service)

A production-grade Platform as a Service (PaaS) engine designed to orchestrate and host isolated Laravel applications using Docker. This platform focuses on administrative control, student resource isolation, and automated deployment workflows.

## Technical Stack

- **Backend:** Go 1.22+ (Fiber, GORM, Redis-Go)
- **Frontend:** React 18+ (Vite, TypeScript, Shadcn UI, Lucide Icons)
- **Orchestration:** Docker Engine (Native API), Traefik v3 (Reverse Proxy & SSL)
- **Databases:** PostgreSQL 15 (Metadata), MariaDB 10.11 (Student Projects)
- **Caching:** Redis 7 (Queue Management & Proxy Cache)

## Core Capabilities

### Deployment Engine
- **Automated Detection:** Analyzes `composer.json` to determine Laravel version and required PHP runtime (8.0 - 8.4).
- **Isolated Runtimes:** Each project runs in a dedicated Nginx+PHP-FPM container managed by Supervisord.
- **Zero-Downtime Swapping:** Deployment worker handles container promotion and old version cleanup gracefully.
- **Stateful Storage:** Persistent volumes for `storage/` and `public/uploads` directories.

### Resource & Lifecycle Management
- **Hard Resource Limits:** Configurable CPU (shares) and Memory (soft/hard limits) via Docker CGroups.
- **Project Expiry:** Automated janitor service for project cleanup after specialized duration.
- **Isolated Databases:** Automatic provisioning of dedicated project databases and users.

### Infrastructure Control
- **Dynamic Proxying:** Real-time subdomain mapping via Traefik and Redis cache-aside pattern.
- **Global Settings:** Centralized dashboard for managing deployment quotas and hardware limits.

## Project Structure

```text
├── backend/               # Go Fiber API (Core Logic)
├── frontend/              # React TypeScript Dashboard
├── docker/
│   ├── templates/         # Dockerfile blueprints (PHP 8.0-8.4)
│   └── traefik/           # Edge Proxy Configuration
├── scripts/
│   ├── start.sh           # Infrastructure initialization script
│   └── nginx-webhook/     # Cross-node synchronization agent
└── storage/               # Native host mount for project data
```

## Setup and Installation

### Prerequisites
- Linux OS (Ubuntu 22.04+ recommended)
- Docker Engine 24.0+
- Public Wildcard DNS record (e.g., `*.paas.example.com`)

### Deployment Steps
1. **Initialize Environment:**
   ```bash
   cp .env.example .env
   # Configure BASE_DOMAIN and infrastructure passwords
   nano .env
   ```

2. **Start Infrastructure:**
   ```bash
   chmod +x scripts/start.sh
   ./scripts/start.sh
   ```

3. **Default Credentials:**
   URL: `http://localhost` (or your configured `BASE_DOMAIN`)
   User: `admin@localhost.com` (Default generated on first init)

## Configuration Reference

Key infrastructure variables located in `.env`:

| Key | Description | Default |
|-----|-------------|---------|
| `JWT_SECRET` | 64-character string for token signing | - |
| `BASE_DOMAIN` | Root domain for dashboard and projects | `localhost` |
| `PROJECTS_PATH` | Host path for git repository storage | `./storage/projects` |
| `DATA_PATH` | Host path for student data persistence | `./storage/data` |
| `REDIS_DATA_PATH` | Host path for Redis persistence | `./storage/redis` |

## API Specification

Detailed API documentation is organized by functional modules:

- **Authentication:** `POST /api/auth/login`, `GET /api/auth/me`
- **Projects:** `GET /api/projects`, `POST /api/projects` (Deploy), `DELETE /api/projects/:id`
- **Database:** `GET /api/projects/:id/database/tables`, `POST /api/projects/:id/database/query`
- **System Admin:** `GET /api/admin/users`, `PUT /api/admin/settings`

## Development

### Running Backend (Development Mode)
```bash
cd backend
go mod tidy
go run cmd/server/main.go
```

### Running Frontend (Vite HMR)
```bash
cd frontend
npm install
npm run dev
```

## License
Copyright (c) 2026. Licensed under the MIT License.
