# Runara

A production-grade, self-hosted platform designed to orchestrate and host isolated, high-performance Laravel applications using Docker. Runara focuses on robust administrative control, developer resource isolation, and automated zero-downtime deployment workflows.

---

## Technical Stack

- **Backend Control Plane:** Go 1.22+ (Fiber, GORM, Redis-Go)
- **Deployment & Sync Worker:** Go 1.22+ (Docker Engine SDK, Redis Streams)
- **Frontend Dashboard:** React 18+ (Vite, TypeScript, TailwindCSS, Shadcn UI, Radix Primitives)
- **Reverse Proxy & Routing:** Traefik v3 (Real-time Dynamic Providers & SSL Provisioning)
- **Platform Database:** PostgreSQL 15 (Metadata, Audit logs, and Outbox Events)
- **Project Database:** MariaDB 10.11 / MySQL (Dedicated tenant databases & isolated users)
- **Cache & Event Bus:** Redis 7 (Queue management, Proxy routing cache, and SSE publication)

---

## Architecture & Project Structure

The project is structured as a Go-based monorepo workspace combined with a modern React dashboard:

```text
├── backend/               # Go Fiber API Control Plane (Handlers, Routes, Services)
├── shared/                # Core Shared Library (Models, Migrations, Repositories, Domain Logic)
│   ├── database/          # Defensive Schema Migrations & Reconciliation
│   ├── models/            # Database Entities (User, Project, CustomDomain, Event logs)
│   └── services/          # Domain Management (Outbox, Setting, Domain State Machine)
├── worker/                # Go Deployment Worker (Docker Lifecycle & Build Engine)
├── frontend/              # React TypeScript Dashboard
│   ├── src/components/    # Reusable UI Primitives (Shadcn + Radix)
│   ├── src/pages/user/    # Developer-facing interfaces (Dashboard, Databases, Domains)
│   └── src/pages/admin/   # Admin Control Center (Users, Systems, Queue, Volume metrics)
├── docker/
│   ├── templates/         # Dockerfile blueprints (PHP 8.0 - 8.4 dynamic builds)
│   └── traefik/           # Traefik dynamic templates & TLS configurations
├── scripts/               # Host shell automation scripts & startup helpers
└── storage/               # Native host mounts for persistent tenant storage
```

---

## Core Capabilities

### 1. Isolated Runtime & Deployment Engine
* **Automated Runtime Detection:** Analyzes the project's `composer.json` to determine the exact PHP runtime needed (PHP 8.0, 8.1, 8.2, 8.3, or 8.4) and boots it with Nginx+PHP-FPM managed by Supervisord.
* **Resource Constraint Enforcement:** Configures hard CPUShares and Memory limits dynamically on tenant containers via Docker CGroups.
* **Zero-Downtime Swapping:** Employs blue-green deployment strategies to swap active containers and clean up legacy builds seamlessly.
* **Stateful Volume Mounting:** Manages persistent host-mapped directories for storage-heavy application assets (`storage/` and `public/uploads`).

### 2. Multi-Tenant Database Isolation
* **Automatic Provisioning:** On project creation, the platform spins up a dedicated MySQL/MariaDB database and user with scoped privileges.
* **Direct Database Management:** A robust built-in browser client allows developers to inspect table schemas, view records, and run custom queries directly from the dashboard.

### 3. Real-Time Dynamic Routing & Custom Domains
* **Zero-Downtime Hot Reloading:** Dynamic subdomains are mapped instantly via Traefik using a Redis cache-aside proxy. Custom domain additions require zero container or proxy restarts.
* **Domain Transfer Engine:** Securely transfer verified custom domains between developer projects with strict validation to prevent hijacking.
* **Reconciliation Engine:** A resilient, goroutine-based reconciler heals routing drift and handles eventual consistency, ensuring Traefik remains perfectly synchronized with the PostgreSQL source of truth.

### 4. Advanced User Roles & Security
* **Access Control Levels:** Segmented into `superadmin` / `admin` (infrastructure, volume telemetry, system parameters) and `user` (general application developers).
* **Impersonation Mode:** Administrators can securely impersonate general developer accounts to troubleshoot application deployment configurations directly.
* **Audit Trail:** Every critical system write, settings update, and domain transfer is tracked inside PostgreSQL audit logs.

---

## Configuration Reference

Key infrastructure variables configured via the root `.env` file:

| Key | Scope | Description | Default |
|-----|-------|-------------|---------|
| `JWT_SECRET` | Platform | 64-character signing key for user authentication tokens | - |
| `BASE_DOMAIN` | Routing | Primary host domain for the platform dashboard and API | `localhost` |
| `PROJECT_DOMAIN` | Routing | Host domain used for project subdomains | `localhost` |
| `PROJECTS_PATH` | Storage | Absolute host path for storing tenant Git repositories | `./storage/projects` |
| `DATA_PATH` | Storage | Absolute host path for tenant volume persistence | `./storage/data` |
| `REDIS_DATA_PATH` | Storage | Absolute host path for Redis AOF/RDB files | `./storage/redis` |

---

## API Specification

The control plane exposes RESTful endpoints for frontend integration:

* **Authentication:** `POST /api/auth/login`, `GET /api/auth/me`, `POST /api/auth/impersonate`
* **Projects:** `GET /api/projects` (List), `POST /api/projects` (Provision), `DELETE /api/projects/:id` (Teardown)
* **Custom Domains:** `GET /api/domains` (List), `POST /api/domains` (Map), `POST /api/domains/transfer` (Migrate)
* **Databases:** `GET /api/projects/:id/database/tables`, `POST /api/projects/:id/database/query`
* **Admin Utilities:** `GET /api/admin/users`, `PUT /api/admin/settings`, `GET /api/admin/containers`

---

## Setup and Installation

### Prerequisites
* Linux Operating System (Ubuntu 22.04 LTS or newer recommended)
* Docker Engine 24.0+ & Docker Compose
* Configured Wildcard DNS A-record (e.g., `*.yourdomain.com` pointing to the VPS IP)

### Getting Started

1. **Initialize the Environment:**
   ```bash
   cp .env.example .env
   # Configure your BASE_DOMAIN, JWT_SECRET, and infrastructure passwords
   nano .env
   ```

2. **Launch Infrastructure & Services:**
   ```bash
   chmod +x scripts/start.sh
   ./scripts/start.sh
   ```

3. **Access the Dashboard:**
   Open `http://localhost` (or your configured `BASE_DOMAIN`) in your browser. The platform auto-provisions a default admin account on initial startup (`admin@localhost.com`).

---

## Development Workflow

### Running Backend (HMR)
```bash
cd backend
go mod tidy
go run cmd/server/main.go
```

### Running Deployment Worker
```bash
cd worker
go mod tidy
go run cmd/worker/main.go
```

### Running Frontend (Vite Dev Server)
```bash
cd frontend
npm install
npm run dev
```

---

## License
Distributed under the MIT License. Copyright &copy; 2026.
