# Nginx Webhook - Komunikasi 2 VPS

## Arsitektur

```
┌─────────────────┐         ┌─────────────────┐
│   VPS PaaS      │         │   VPS Nginx     │
│  (Laravel PaaS) │  HTTP   │  (IP Public)    │
│                 │ ──────► │                 │
│  webhook-client │         │  webhook.sh     │
└─────────────────┘         └─────────────────┘
```

## Setup VPS Nginx (IP Public)

1. **Install dependencies:**
   ```bash
   apt update
   apt install socat certbot nginx
   ```

2. **Copy webhook script:**
   ```bash
   mkdir -p /opt/nginx-webhook
   cp webhook.sh /opt/nginx-webhook/
   chmod +x /opt/nginx-webhook/webhook.sh
   ```

3. **Edit config:**
   ```bash
   nano /opt/nginx-webhook/nginx-webhook.service
   # Ganti:
   #   WEBHOOK_SECRET=your-secret-key-here
   #   BASE_DOMAIN=smkmuh1bantul.sch.id
   ```

4. **Install service:**
   ```bash
   cp nginx-webhook.service /etc/systemd/system/
   systemctl daemon-reload
   systemctl enable nginx-webhook
   systemctl start nginx-webhook
   ```

5. **Buat directory untuk sites:**
   ```bash
   mkdir -p /etc/nginx/sites-available/paas
   ```

6. **Pastikan proxy_params dan ssl_params ada:**
   ```bash
   # /etc/nginx/proxy_params (jika belum ada)
   cat > /etc/nginx/proxy_params << 'EOF'
   proxy_set_header Host $host;
   proxy_set_header X-Real-IP $remote_addr;
   proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
   proxy_set_header X-Forwarded-Proto $scheme;
   proxy_http_version 1.1;
   proxy_set_header Upgrade $http_upgrade;
   proxy_set_header Connection "upgrade";
   EOF
   ```

## Setup VPS PaaS

1. **Copy client script:**
   ```bash
   cp webhook-client.sh /usr/local/bin/
   chmod +x /usr/local/bin/webhook-client.sh
   ```

2. **Set environment:**
   ```bash
   export WEBHOOK_URL="http://192.168.255.1:5000"  # IP VPS Nginx
   export WEBHOOK_SECRET="your-secret-key-here"   # Same as server
   ```

## Usage

**Create project:**
```bash
webhook-client.sh create my-project 192.168.255.114 3001
```

**Delete project:**
```bash
webhook-client.sh delete my-project
```

**Health check:**
```bash
webhook-client.sh health
```

## API Endpoints

| Method | Endpoint | Body | Description |
|--------|----------|------|-------------|
| POST | /webhook/create | `{subdomain, backend_ip, port}` | Create nginx + SSL |
| POST | /webhook/update | `{subdomain, backend_ip, port}` | Update config |
| POST | /webhook/delete | `{subdomain}` | Remove config |
| GET | /webhook/health | - | Health check |

## Security

- Webhook hanya listen di port internal (5000)
- Gunakan firewall untuk block akses dari luar:
  ```bash
  ufw allow from 192.168.255.0/24 to any port 5000
  ```
- Secret key untuk autentikasi
