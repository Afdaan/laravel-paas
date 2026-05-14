# Nginx Webhook Proxy (Python Version)

This script acts as a bridge between the **Laravel PaaS Backend** and the **Remote Nginx VPS**. It automatically manages Nginx configurations and SSL certificates (via Certbot) when projects are created or deleted.

## 🚀 Installation on Nginx VPS

### 1. Prerequisites
Ensure your Nginx VPS has Python 3 and Certbot installed:
```bash
sudo apt update
sudo apt install python3-pip python3-venv certbot python3-certbot-nginx -y
```

### 2. Setup Directory
```bash
sudo mkdir -p /opt/paas-webhook
sudo chown $USER:$USER /opt/paas-webhook
# Copy files from this directory to /opt/paas-webhook on the VPS
```

### 3. Create Virtual Environment & Install Dependencies
```bash
cd /opt/paas-webhook
python3 -m venv venv
source venv/bin/activate
pip install flask python-dotenv
```

### 4. Configuration
Create a `.env` file based on `.env.example`:
```bash
cp .env.example .env
nano .env
```
Make sure `WEBHOOK_KEY` matches the `NGINX_WEBHOOK_KEY` in your PaaS Backend `.env`.

### 5. Deployment with Systemd
Copy the service file to systemd:
```bash
sudo cp paas-webhook.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable paas-webhook
sudo systemctl start paas-webhook
```

## 🛠 Features
- **Auto-SSL**: Automatically provisions Let's Encrypt certificates.
- **Port Detection**: Proxies traffic to the correct internal port of the PaaS VPS.
- **Security**: Authorized via shared secret key.
- **Monorepo Support**: Organizes configs into subdirectories per user.
- **Rate Limiting**: Includes built-in Nginx rate limiting templates.
