from flask import Flask, request, jsonify
import os
import subprocess
import logging
from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()

app = Flask(__name__)

# Configuration
NGINX_CONF_DIR = "/etc/nginx/conf.d/paas-hosting"
SSL_EMAIL = os.environ.get("SSL_EMAIL", "admin@example.com")
WEBHOOK_KEY = os.environ.get("WEBHOOK_KEY", "change-this-key")
CERTBOT_COMMAND = "certbot certonly --nginx --non-interactive --agree-tos -m {email} -d {domain}"

# Ensure log directory exists
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

def run_command(command):
    try:
        logging.info(f"Executing: {command}")
        result = subprocess.run(command, shell=True, check=True, capture_output=True, text=True)
        return True, result.stdout
    except subprocess.CalledProcessError as e:
        logging.error(f"Command failed: {e.stderr}")
        return False, e.stderr


@app.route('/webhook', methods=['POST'])
def webhook():
    # Security: Check API Key
    key = request.headers.get("X-Webhook-Key")
    if not key or key != WEBHOOK_KEY:
        logging.warning(f"Unauthorized access attempt with key: {key}")
        return jsonify({"error": "Unauthorized"}), 401
    
    data = request.json
    if not data:
        return jsonify({"error": "No data provided"}), 400

    action = data.get("action")
    subdomain = data.get("subdomain")
    domain = data.get("domain")
    user_folder = data.get("user_folder", "project-student")

    if not action or not subdomain or not domain:
        return jsonify({"error": "Missing required fields"}), 400

    # Project directory inside paas-hosting
    project_dir = os.path.join(NGINX_CONF_DIR, user_folder)
    os.makedirs(project_dir, exist_ok=True)

    if action == "sync":
        internal_ip = data.get("internal_ip")
        port = data.get("port")
        
        if not internal_ip or not port:
            return jsonify({"error": "Missing internal_ip or port for sync"}), 400

        # Create/Update config
        conf_content = f"""server {{
    listen 80;
    server_name {domain};

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
        
        logging.info(f"Nginx config created for {subdomain} at {file_path}")
        
        # Reload Nginx
        success, _ = run_command("nginx -s reload")
        if not success:
            return jsonify({"error": "Failed to reload Nginx"}), 500

        # Try to generate/activate SSL
        logging.info(f"Attempting SSL for {domain}")
        ssl_success, _ = run_command(CERTBOT_COMMAND.format(email=SSL_EMAIL, domain=domain))
        
        return jsonify({"message": f"Project {subdomain} synced successfully"}), 200

    elif action == "delete":
        file_path = os.path.join(project_dir, f"project-{subdomain}.conf")
        if os.path.exists(file_path):
            os.remove(file_path)
            logging.info(f"Deleted Nginx config for {subdomain}")
            
            # Reload Nginx
            run_command("nginx -s reload")
            
            return jsonify({"message": f"Project {subdomain} deleted successfully"}), 200
        else:
            return jsonify({"message": f"Project {subdomain} config not found, nothing to delete"}), 200

    return jsonify({"error": "Invalid action"}), 400

if __name__ == '__main__':
    # Listen on all interfaces
    app.run(host='0.0.0.0', port=49512)
