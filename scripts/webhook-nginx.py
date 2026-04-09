from flask import Flask, request, jsonify
import os
import subprocess
import logging
from logging.handlers import TimedRotatingFileHandler
from dotenv import load_dotenv

load_dotenv()

app = Flask(__name__)

# Configuration
NGINX_CONF_DIR = os.environ.get("NGINX_CONF_DIR", "/etc/nginx/conf.d/paas-hosting")
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

def sync_project(subdomain, domain, internal_ip, port, project_dir):
    """
    Generates Nginx configuration and provisions SSL for a project.
    """
    conf_content = f"""server {{
    listen 80;
    server_name {domain};

    # Serve ACME challenge files locally for Certbot HTTP-01 validation
    location /.well-known/acme-challenge/ {{
        root /var/www/html;
    }}

    location / {{
        proxy_pass http://{internal_ip}:{port};
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }}
}}
"""
    file_path = os.path.join(project_dir, f"project-{subdomain}.conf")
    
    with open(file_path, "w") as f:
        f.write(conf_content)
    
    logging.info(f"[subdomain: {subdomain}] Created Nginx configuration at {file_path}")
    
    success, _ = run_command(["nginx", "-s", "reload"])
    if not success:
        return False, "Failed to reload Nginx"

    logging.info(f"[subdomain: {subdomain}] Attempting SSL provisioning for {domain}")
    certbot_args = [
        "certbot", "--nginx", "--non-interactive", "--agree-tos",
        "-m", SSL_EMAIL, "-d", domain, "--redirect"
    ]
    
    ssl_success, _ = run_command(certbot_args)
    if not ssl_success:
        logging.warning(f"[subdomain: {subdomain}] SSL generation failed, but HTTP config is active.")
        
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
    client_ip = request.remote_addr
    key = request.headers.get("X-Webhook-Key")
    
    if not key or key != WEBHOOK_KEY:
        logging.warning(f"[src_ip: {client_ip}] Unauthorized webhook access attempt; Key validation failed")
        return jsonify({"error": "Unauthorized"}), 401
    
    # Force JSON parsing even if Content-Type header from backend is missing/distorted
    data = request.get_json(force=True, silent=True)
    if not data:
        raw_data = request.data.decode('utf-8')
        logging.error(f"[src_ip: {client_ip}] Invalid payload: Raw data received: '{raw_data}'")
        return jsonify({"error": "Invalid payload"}), 400

    action = data.get("action")
    subdomain = data.get("subdomain")
    domain = data.get("domain")
    user_folder = data.get("user_folder", "project-student")

    logging.info(f"[action: {action}] [subdomain: {subdomain}] Webhook process started from {client_ip}")

    if not action or not subdomain or not domain:
        logging.error(f"[action: {action}] Process aborted: Missing required core fields (action/subdomain/domain)")
        return jsonify({"error": "Missing required core fields"}), 400

    project_dir = os.path.join(NGINX_CONF_DIR, user_folder)
    os.makedirs(project_dir, exist_ok=True)

    if action == "sync":
        internal_ip = data.get("internal_ip")
        port = data.get("port")
        
        if not internal_ip or not port:
            logging.error(f"[action: {action}] [subdomain: {subdomain}] Process aborted: Missing internal_ip or port")
            return jsonify({"error": "Missing internal_ip or port for synchronization"}), 400

        logging.info(f"[action: {action}] [subdomain: {subdomain}] Target config: Domain={domain}, Proxy={internal_ip}:{port}, Folder={user_folder}")
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
