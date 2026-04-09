from flask import Flask, request, jsonify
import os
import subprocess
import logging
import threading
from logging.handlers import TimedRotatingFileHandler
from dotenv import load_dotenv

load_dotenv()

app = Flask(__name__)

# Configuration
NGINX_CONF_DIR = os.environ.get("NGINX_CONF_DIR", "/etc/nginx/conf.d/paas-hosting")
NGINX_LIMIT_CONF = os.path.join(os.path.dirname(NGINX_CONF_DIR), "paas-rate-limits.conf")
SSL_EMAIL = os.environ.get("SSL_EMAIL", "admin@example.com")
WEBHOOK_KEY = os.environ.get("WEBHOOK_KEY", "change-this-key")

# Logging Configuration
BASE_DIR = os.path.dirname(os.path.abspath(__file__))
LOG_FILE = os.path.join(BASE_DIR, "webhook.log")

file_handler = TimedRotatingFileHandler(LOG_FILE, when="W0", interval=1, backupCount=4)
file_handler.setLevel(logging.INFO)

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - [%(funcName)s] - %(message)s',
    handlers=[file_handler, logging.StreamHandler()]
)

# Global lock to prevent concurrent Nginx/Certbot operations
GLOBAL_SYNC_LOCK = threading.Lock()

def ensure_global_limits():
    """
    Ensures that global rate limit and connection limit zones are defined.
    These settings protect the server from being overwhelmed by a single IP.
    """
    if not os.path.exists(NGINX_LIMIT_CONF):
        content = """
# Request Rate Limiting (10 requests per second with a burst of 20)
limit_req_zone $binary_remote_addr zone=paas_req_limit:10m rate=10r/s;

# Connection Limiting (Max 20 concurrent connections per IP)
limit_conn_zone $binary_remote_addr zone=paas_conn_limit:10m;
"""
        try:
            # We don't use run_command here because we're writing a new file
            with open(NGINX_LIMIT_CONF, "w") as f:
                f.write(content)
            logging.info(f"Initialized global rate limit definitions at {NGINX_LIMIT_CONF}")
        except Exception as e:
            logging.error(f"Failed to initialize global limits: {str(e)}")

def run_command(command_args):
    """
    Executes a shell command securely without shell=True to prevent injection.
    """
    cmd_str = ' '.join(command_args)
    try:
        logging.info(f"Executing command: {cmd_str}")
        result = subprocess.run(command_args, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)
        # Log purely debug info, keeping info logs uncluttered
        logging.info(f"Command success: {cmd_str}")
        return True, result.stdout
    except subprocess.CalledProcessError as e:
        error_msg = e.stderr.strip() if e.stderr else e.stdout.strip()
        logging.error(f"Command execution failed: {cmd_str} | Error: {error_msg}")
        return False, e.stderr

def get_nginx_config(domain, internal_ip, port, ssl_enabled=False):
    """
    Generates Nginx configuration content with security and performance limits.
    """
    # Security and Performance directives
    # limit_req: Throttles requests
    # limit_conn: Prevents too many simultaneous connections
    limits_config = """
        # Apply Rate Limiting
        limit_req zone=paas_req_limit burst=20 nodelay;
        limit_conn paas_conn_limit 20;

        # Client body limits for security
        client_body_buffer_size 128k;
    """

    proxy_config = f"""
        {limits_config}
        # Proxy settings to internal IP
        proxy_pass http://{internal_ip}:{port};
        
        # Force Laravel to detect HTTPS
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-Port 443;

        # Automatically redirect HTTP from backend to HTTPS
        proxy_redirect http:// $scheme://;

        # WebSocket Support
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # Standard Reverse Proxy Headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        # CSP for Mixed Content
        add_header Content-Security-Policy "upgrade-insecure-requests";

        # Session & Cookie Persistence
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Server $host;
        proxy_buffer_size 128k;
        proxy_buffers 4 256k;
        proxy_busy_buffers_size 256k;

        # Timeouts (Generous for student apps but capped at 5 mins)
        proxy_connect_timeout 300;
        proxy_send_timeout 300;
        proxy_read_timeout 300;
        send_timeout 300;
    """

    if not ssl_enabled:
        return f"""server {{
    listen 80;
    server_name {domain};
    client_max_body_size 64M;

    location /.well-known/acme-challenge/ {{
        root /var/www/html;
    }}

    location / {{{proxy_config}    }}
}}
"""
    else:
        return f"""server {{
    listen 80;
    server_name {domain};
    return 301 https://$host$request_uri;
}}

server {{
    listen 443 ssl;
    server_name {domain};
    client_max_body_size 64M;

    ssl_certificate /etc/letsencrypt/live/{domain}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/{domain}/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;

    location / {{{proxy_config}    }}
}}
"""

def sync_project(subdomain, domain, internal_ip, port, project_dir):
    """
    Generates Nginx configuration and provisions SSL for a project.
    Optimized to skip Certbot if certificate already exists.
    """
    cert_path = f"/etc/letsencrypt/live/{domain}/fullchain.pem"
    options_ssl_path = "/etc/letsencrypt/options-ssl-nginx.conf"
    
    # Check if certificate already exists
    ssl_previously_provisioned = os.path.exists(cert_path)
    
    # Check if we have the shared SSL options (usually created by first Certbot run)
    has_ssl_options = os.path.exists(options_ssl_path)
    
    # If cert exists but options don't (rare case), we still need Certbot to fix it
    use_manual_ssl = ssl_previously_provisioned and has_ssl_options
    
    conf_content = get_nginx_config(domain, internal_ip, port, ssl_enabled=use_manual_ssl)
    file_path = os.path.join(project_dir, f"project-{subdomain}.conf")
    
    with open(file_path, "w") as f:
        f.write(conf_content)
    
    logging.info(f"[subdomain: {subdomain}] Nginx configuration written (SSL: {use_manual_ssl})")
    
    success, _ = run_command(["nginx", "-s", "reload"])
    if not success:
        return False, "Failed to reload Nginx"

    # Only run Certbot if SSL is not yet provisioned
    if not use_manual_ssl:
        logging.info(f"[subdomain: {subdomain}] Certbot required. Attempting SSL provisioning for {domain}")
        certbot_args = [
            "certbot", "--nginx", "--non-interactive", "--agree-tos",
            "-m", SSL_EMAIL, "-d", domain, "--redirect"
        ]
        
        ssl_success, _ = run_command(certbot_args)
        if not ssl_success:
            logging.warning(f"[subdomain: {subdomain}] SSL generation failed. HTTP is active.")
        else:
            logging.info(f"[subdomain: {subdomain}] Certbot successfully provisioned SSL.")
    else:
        logging.info(f"[subdomain: {subdomain}] Re-used existing SSL certificate. Skipped Certbot.")
        
    return True, f"Project {subdomain} synced successfully"

def delete_project(subdomain, project_dir):
    """
    Removes Nginx configuration for a project.
    """
    file_path = os.path.join(project_dir, f"project-{subdomain}.conf")
    
    if os.path.exists(file_path):
        os.remove(file_path)
        logging.info(f"[subdomain: {subdomain}] Deleted Nginx configuration")
        
        run_command(["nginx", "-s", "reload"])
        return True, f"Project {subdomain} deleted successfully"
        
    logging.info(f"[subdomain: {subdomain}] Configuration not found, no deletion required")
    return True, f"Project {subdomain} config not found"

@app.route('/webhook', methods=['POST'])
def webhook():
    """
    Webhook entry point for project synchronization and deletion.
    Wrapped in GLOBAL_SYNC_LOCK to ensure serial execution.
    """
    client_ip = request.remote_addr
    key = request.headers.get("X-Webhook-Key")
    
    if not key or key != WEBHOOK_KEY:
        logging.warning(f"[src_ip: {client_ip}] Unauthorized access attempt")
        return jsonify({"error": "Unauthorized"}), 401
    
    data = request.get_json(force=True, silent=True)
    if not data:
        return jsonify({"error": "Invalid payload"}), 400

    action = data.get("action")
    subdomain = data.get("subdomain")
    domain = data.get("domain")
    user_folder = data.get("user_folder", "project-student")

    if not action or not subdomain or not domain:
        return jsonify({"error": "Missing core fields"}), 400

    # Acquire lock to prevent race conditions during Nginx reloads or Certbot runs
    with GLOBAL_SYNC_LOCK:
        logging.info(f"[action: {action}] [subdomain: {subdomain}] Processing webhook from {client_ip}")

        # Ensure global settings exist before writing project configs
        ensure_global_limits()

        project_dir = os.path.join(NGINX_CONF_DIR, user_folder)
        os.makedirs(project_dir, exist_ok=True)

        if action == "sync":
            internal_ip = data.get("internal_ip")
            port = data.get("port")
            
            if not internal_ip or not port:
                return jsonify({"error": "Missing internal_ip or port"}), 400

            success, message = sync_project(subdomain, domain, internal_ip, port, project_dir)
            if not success:
                return jsonify({"error": message}), 500
                
            return jsonify({"message": message}), 200

        elif action == "delete":
            success, message = delete_project(subdomain, project_dir)
            return jsonify({"message": message}), 200

    return jsonify({"error": "Unknown action type"}), 400

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=49512)
