# Laravel PaaS

A Platform as a Service for hosting Laravel applications with Docker. Designed for schools and universities.

![Laravel](https://img.shields.io/badge/Laravel-8%20%7C%209%20%7C%2010%20%7C%2011-FF2D20?style=flat-square&logo=laravel)
![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat-square&logo=go)
![React](https://img.shields.io/badge/React-18-61DAFB?style=flat-square&logo=react)
![Docker](https://img.shields.io/badge/Docker-24+-2496ED?style=flat-square&logo=docker)

## Features

### Student Dashboard
- Deploy projects from GitHub URL
- Monitor CPU & Memory usage
- View container logs
- Database Manager (browse tables, run queries, export/import SQL)
- Redeploy & delete projects

### Admin Dashboard
- User management (CRUD)
- Import students from Excel
- Global settings (limits, expiry, domain)
- Overview of all projects

### Technical Features
- **Auto Laravel Detection** - Detects Laravel version from `composer.json`
- **Multi PHP Support** - PHP 8.0, 8.1, 8.2, 8.3
- **Auto SSL** - Via Traefik + Let's Encrypt
- **Database Per Project** - Isolated MySQL database
- **Resource Limits** - CPU & memory limits per container

## Requirements

- Docker Engine 24+
- Docker Compose (optional)
- Domain with wildcard DNS (for production)

## Quick Start

```bash
# Clone repository
git clone https://github.com/yourusername/laravel-paas.git
cd laravel-paas

# Copy environment file
cp .env.example .env

# Edit configuration
nano .env

# Start the platform
chmod +x scripts/start.sh
./scripts/start.sh
```

**Default Login:** `admin@localhost` / `admin123`

## Project Structure

```
laravel-paas/
├── frontend/              # React + Vite + TailwindCSS
│   ├── src/
│   │   ├── pages/         # Student & Admin pages
│   │   ├── components/    # Reusable components
│   │   ├── services/      # API client
│   │   └── stores/        # Zustand state management
│   └── Dockerfile
│
├── backend/               # Go + Fiber API
│   ├── cmd/server/        # Entry point
│   ├── internal/
│   │   ├── handlers/      # HTTP handlers
│   │   ├── services/      # Business logic (Docker, Nginx)
│   │   ├── models/        # GORM models
│   │   └── middleware/    # JWT auth
│   └── Dockerfile
│
├── docker/
│   ├── templates/         # Laravel Dockerfile templates (PHP 8.0-8.3)
│   │   ├── Dockerfile.php80
│   │   ├── Dockerfile.php81
│   │   ├── Dockerfile.php82
│   │   ├── Dockerfile.php83
│   │   ├── nginx.conf
│   │   └── supervisord.conf
│   └── traefik/           # Reverse proxy config
│
├── scripts/
│   ├── start.sh           # Start all services
│   ├── stop.sh            # Stop all services
│   └── nginx-webhook/     # Webhook for multi-VPS setup
│
└── storage/projects/      # Cloned student repositories
```

## Configuration

- Built-in UI code editor to see / manage project environment variables

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MYSQL_ROOT_PASSWORD` | MySQL root password | - |
| `MYSQL_DATABASE` | Database name | `paas` |
| `JWT_SECRET` | JWT signing secret | - |
| `BASE_DOMAIN` | Base domain for projects | `localhost` |
| `ACME_EMAIL` | Email for Let's Encrypt | - |
| `DEFAULT_MAX_PROJECTS` | Max projects per user | `3` |
| `DEFAULT_EXPIRY_DAYS` | Project expiry days | `30` |
| `DEFAULT_CPU_LIMIT` | CPU limit per container | `0.5` |
| `DEFAULT_MEMORY_LIMIT` | Memory limit | `512m` |

## Architecture

```
                                    ┌─────────────────┐
                                    │   Traefik       │
                                    │  (SSL + Proxy)  │
                                    └────────┬────────┘
                                             │
              ┌──────────────────────────────┼──────────────────────────────┐
              │                              │                              │
    ┌─────────▼─────────┐        ┌───────────▼───────────┐       ┌──────────▼──────────┐
    │  Frontend (React) │        │  Backend (Go + Fiber) │       │  Student Projects   │
    │     Port 80       │        │       Port 8080       │       │  Port 3001-4000     │
    └───────────────────┘        └───────────┬───────────┘       └─────────────────────┘
                                             │
                          ┌──────────────────┼──────────────────┐
                          │                  │                  │
                ┌─────────▼─────────┐  ┌─────▼─────┐  ┌─────────▼─────────┐
                │  MySQL Database   │  │   Redis   │  │   Docker Daemon   │
                │   (paas + dbs)    │  │  (cache)  │  │ (container mgmt)  │
                └───────────────────┘  └───────────┘  └───────────────────┘
```

## API Endpoints

### Authentication
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/auth/login` | Login |
| POST | `/api/auth/logout` | Logout |
| GET | `/api/auth/me` | Get current user |

### Projects (Student)
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/projects` | List own projects |
| POST | `/api/projects` | Deploy new project |
| GET | `/api/projects/:id` | Get project details |
| POST | `/api/projects/:id/redeploy` | Redeploy project |
| DELETE | `/api/projects/:id` | Delete project |
| GET | `/api/projects/:id/logs` | Get container logs |
| GET | `/api/projects/:id/stats` | Get resource stats |

### Database Manager
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/projects/:id/database/tables` | List tables |
| GET | `/api/projects/:id/database/tables/:table/data` | Get table data |
| POST | `/api/projects/:id/database/query` | Execute SQL query |
| GET | `/api/projects/:id/database/export` | Export as SQL file |
| POST | `/api/projects/:id/database/import` | Import SQL |

### Admin
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/admin/users` | List users |
| POST | `/api/admin/users` | Create user |
| POST | `/api/admin/users/import` | Import from Excel |
| GET | `/api/admin/settings` | Get settings |
| PUT | `/api/admin/settings` | Update settings |

## Development

### Backend
```bash
cd backend
go mod tidy
go run cmd/server/main.go
```

### Frontend
```bash
cd frontend
npm install
npm run dev
```

## License

MIT License - Feel free to use for educational purposes.

## Contributing

Contributions welcome! Please open an issue first to discuss changes.

---

Made with passion for education
