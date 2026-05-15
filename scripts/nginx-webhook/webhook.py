import os
import subprocess
import logging
import threading
from logging.handlers import TimedRotatingFileHandler
from flask import Flask, request, jsonify
from dotenv import load_dotenv

# Initialize environment and app
load_dotenv()
app = Flask(__name__)

# --- Configuration ---
NGINX_CONF_DIR = os.environ.get("NGINX_CONF_DIR", "/etc/nginx/conf.d/paas-hosting")
NGINX_LIMIT_CONF = os.path.join(os.path.dirname(NGINX_CONF_DIR), "paas-rate-limits.conf")
SSL_EMAIL = os.environ.get("SSL_EMAIL", "admin@example.com")
WEBHOOK_KEY = os.environ.get("WEBHOOK_KEY", "change-this-key")
LISTEN_PORT = 49512

# --- Logging Configuration ---
BASE_DIR = os.path.dirname(os.path.abspath(__file__))
LOG_FILE = os.path.join(BASE_DIR, "webhook.log")

file_handler = TimedRotatingFileHandler(LOG_FILE, when="W0", interval=1, backupCount=4)
file_handler.setLevel(logging.INFO)

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - [%(funcName)s] - %(message)s',
    handlers=[file_handler, logging.StreamHandler()]
)

# Global lock for serializing Nginx/Certbot operations
GLOBAL_SYNC_LOCK = threading.Lock()

# --- Nginx Configuration Templates ---

PROXY_DIRECTIVES_TEMPLATE = """
        # Rate & Connection Limiting
        limit_req zone=paas_req_limit burst=20 nodelay;
        limit_conn paas_conn_limit 20;

        # Client Body & Header Buffers
        client_body_buffer_size 256k;
        proxy_buffer_size 128k;
        proxy_buffers 4 256k;
        proxy_busy_buffers_size 256k;

        # Proxy settings to internal IP
        proxy_pass http://{internal_ip}:{port};
        
        # HTTPS Detection for Backend (Laravel)
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-Port 443;
        proxy_redirect http:// $scheme://;

        # WebSocket Support
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # Standard Proxy Headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Server $host;

        # Security Headers
        add_header Content-Security-Policy "upgrade-insecure-requests";
        add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

        # Performance & Reliability
        gzip on;
        gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript image/svg+xml;

        proxy_next_upstream error timeout http_502 http_503 http_504;
        proxy_next_upstream_tries 3;
        proxy_next_upstream_timeout 10s;

        # Timeouts
        proxy_connect_timeout 300;
        proxy_send_timeout 300;
        proxy_read_timeout 300;
        send_timeout 300;
"""

COMMON_SERVER_DIRECTIVES = """
    client_max_body_size 64M;

    # Silent handling for common missing files
    location = /favicon.ico { access_log off; log_not_found off; }
    location = /robots.txt  { access_log off; log_not_found off; }
"""

def ensure_global_limits():
    """Initializes global rate limit zones if missing."""
    if os.path.exists(NGINX_LIMIT_CONF):
        return

    content = """
# Request Rate Limiting (10 requests per second)
limit_req_zone $binary_remote_addr zone=paas_req_limit:10m rate=10r/s;

# Connection Limiting (Max 20 concurrent connections)
limit_conn_zone $binary_remote_addr zone=paas_conn_limit:10m;
"""
    try:
        with open(NGINX_LIMIT_CONF, "w") as f:
            f.write(content)
        logging.info(f"Initialized global rate limits at {NGINX_LIMIT_CONF}")
    except Exception as e:
        logging.error(f"Failed to initialize global limits: {str(e)}")

def run_command(command_args):
    """Executes shell commands without shell=True for security."""
    cmd_str = ' '.join(command_args)
    try:
        logging.info(f"Executing: {cmd_str}")
        subprocess.run(command_args, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)
        return True, "Success"
    except subprocess.CalledProcessError as e:
        error_msg = e.stderr.strip() if e.stderr else e.stdout.strip()
        logging.error(f"Command failed: {cmd_str} | Error: {error_msg}")
        return False, error_msg

def get_nginx_config(all_domains_str, internal_ip, port, ssl_enabled=False, primary_domain=None):
    """Generates the full Nginx configuration string."""
    proxy_config = PROXY_DIRECTIVES_TEMPLATE.format(internal_ip=internal_ip, port=port)
    
    if not ssl_enabled:
        return f"""server {{
    listen 80;
    server_name {all_domains_str};
    {COMMON_SERVER_DIRECTIVES}

    location /.well-known/acme-challenge/ {{
        root /var/www/html;
    }}

    location / {{
{proxy_config}
    }}
}}
"""

    return f"""server {{
    listen 80;
    server_name {all_domains_str};
    return 301 https://$host$request_uri;
}}

server {{
    listen 443 ssl;
    server_name {all_domains_str};
    {COMMON_SERVER_DIRECTIVES}

    ssl_certificate /etc/letsencrypt/live/{primary_domain}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/{primary_domain}/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;

    location / {{
{proxy_config}
    }}
}}
"""

def sync_project(subdomain, domain, custom_domains, internal_ip, port, project_dir):
    """Handles project Nginx configuration and SSL provisioning."""
    
    # Combine domains for Nginx config
    all_domains_list = [domain] + custom_domains
    all_domains_str = " ".join(all_domains_list)
    
    # We check SSL on the primary domain. If any custom domains are added, 
    # certbot --expand will add them to the existing certificate later.
    cert_path = f"/etc/letsencrypt/live/{domain}/fullchain.pem"
    options_ssl_path = "/etc/letsencrypt/options-ssl-nginx.conf"
    
    has_cert = os.path.exists(cert_path)
    has_ssl_options = os.path.exists(options_ssl_path)
    
    # Logic: Prioritize existing SSL, but repair if options are missing
    use_manual_ssl = has_cert
    if use_manual_ssl and not has_ssl_options:
        logging.warning(f"[{subdomain}] Cert exists but SSL options missing. Retrying Certbot.")
        use_manual_ssl = False

    file_path = os.path.join(project_dir, f"project-{subdomain}.conf")
    
    # Backup old config for rollback
    old_content = None
    if os.path.exists(file_path):
        with open(file_path, "r") as f:
            old_content = f.read()

    conf_content = get_nginx_config(all_domains_str, internal_ip, port, ssl_enabled=use_manual_ssl, primary_domain=domain)
    
    try:
        with open(file_path, "w") as f:
            f.write(conf_content)
        
        # VERIFY: Run nginx -t before reloading
        test_success, test_out = run_command(["nginx", "-t"])
        if not test_success:
            logging.error(f"[{subdomain}] Nginx syntax test failed! Rolling back. Error: {test_out}")
            if old_content:
                with open(file_path, "w") as f:
                    f.write(old_content)
            else:
                if os.path.exists(file_path):
                    os.remove(file_path)
            return False, f"Nginx syntax error: {test_out}"

        # RELOAD: Only if syntax is valid
        success, _ = run_command(["nginx", "-s", "reload"])
        if not success:
            return False, "Nginx reload failed"
            
    except Exception as e:
        logging.exception(f"[{subdomain}] Critical error during sync")
        return False, str(e)

    if use_manual_ssl:
        logging.info(f"[{subdomain}] Sync complete using existing SSL.")
        return True, "Synced with existing SSL"

    # Provision new SSL or Expand existing
    logging.info(f"[{subdomain}] Provisioning/Expanding SSL certificate for {all_domains_str}")
    
    certbot_args = [
        "certbot", "--nginx", "--non-interactive", "--agree-tos",
        "-m", SSL_EMAIL, "--cert-name", domain, "--expand", "--redirect"
    ]
    
    for d in all_domains_list:
        certbot_args.extend(["-d", d])
    
    ssl_success, _ = run_command(certbot_args)
    if not ssl_success:
        logging.warning(f"[{subdomain}] SSL provisioning failed. HTTP active.")
        return True, "Synced (HTTP only)"
    
    return True, "Synced with new SSL"

def delete_project(subdomain, project_dir):
    """Cleans up project Nginx configuration."""
    file_path = os.path.join(project_dir, f"project-{subdomain}.conf")
    
    if not os.path.exists(file_path):
        return True, "Not found"
        
    os.remove(file_path)
    run_command(["nginx", "-s", "reload"])
    logging.info(f"[{subdomain}] Deleted configuration")
    return True, "Deleted"

@app.route('/webhook', methods=['POST'])
def webhook():
    """Main webhook entry point."""
    client_ip = request.remote_addr
    auth_key = request.headers.get("X-Webhook-Key")
    
    if not auth_key or auth_key != WEBHOOK_KEY:
        logging.warning(f"Unauthorized access from {client_ip}")
        return jsonify({"error": "Unauthorized"}), 401
    
    data = request.get_json(force=True, silent=True) or {}
    action = data.get("action")
    subdomain = data.get("subdomain")
    domain = data.get("domain")
    custom_domains = data.get("custom_domains", [])
    user_folder = data.get("user_folder", "project-student")

    if not all([action, subdomain, domain]):
        return jsonify({"error": "Missing required fields"}), 400

    with GLOBAL_SYNC_LOCK:
        ensure_global_limits()
        project_dir = os.path.join(NGINX_CONF_DIR, user_folder)
        os.makedirs(project_dir, exist_ok=True)

        if action == "sync":
            internal_ip = data.get("internal_ip")
            port = data.get("port")
            if not internal_ip or not port:
                return jsonify({"error": "Missing IP/Port"}), 400
            
            success, message = sync_project(subdomain, domain, custom_domains, internal_ip, port, project_dir)
            return jsonify({"message": message}), 200 if success else 500

        if action == "delete":
            success, message = delete_project(subdomain, project_dir)
            return jsonify({"message": message}), 200

    return jsonify({"error": "Invalid action"}), 400

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=LISTEN_PORT)
