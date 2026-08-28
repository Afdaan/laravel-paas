import os
import subprocess
import logging
import threading
import hashlib
import queue
import hmac
import time
import email.utils
import re
from logging.handlers import TimedRotatingFileHandler
from flask import Flask, request, jsonify
from dotenv import load_dotenv

def format_openssl_date(date_str):
    if not date_str:
        return None
    try:
        dt = email.utils.parsedate_to_datetime(date_str)
        return dt.isoformat()
    except Exception as e:
        logging.error(f"Failed to parse openssl date '{date_str}': {str(e)}")
        return date_str


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

# Global lock for serializing Nginx file operations
GLOBAL_SYNC_LOCK = threading.Lock()

# --- Asynchronous Workers & Reloader ---

class NginxReloader:
    """Debounces Nginx reloads using a sliding window to prevent reload storms during bulk reconciliation updates."""
    def __init__(self):
        self.lock = threading.Lock()
        self.timer = None
        self.first_request_time = None
        self.total_reloads = 0
        self.skipped_reloads = 0
        self.pending_reloads = 0
        self.last_reload_time = None
        self.debounce_window = 1.0
        self.max_window = 3.0

    def schedule_reload(self):
        with self.lock:
            now = time.time()
            if self.timer is None:
                self.first_request_time = now
                self.pending_reloads = 1
                self.timer = threading.Timer(self.debounce_window, self._execute_reload)
                self.timer.start()
            else:
                self.pending_reloads += 1
                self.skipped_reloads += 1
                if now - self.first_request_time < (self.max_window - self.debounce_window):
                    self.timer.cancel()
                    self.timer = threading.Timer(self.debounce_window, self._execute_reload)
                    self.timer.start()

    def _execute_reload(self):
        with self.lock:
            self.timer = None
            self.first_request_time = None
            pending = self.pending_reloads
            self.pending_reloads = 0
            self.total_reloads += 1
            self.last_reload_time = time.time()

        logging.info(f"Executing coalesced Nginx reload across cluster (coalesced {pending} updates into 1 reload)...")
        run_command(["nginx", "-s", "reload"])

    def get_metrics(self):
        with self.lock:
            return {
                "total_reloads": self.total_reloads,
                "skipped_reloads": self.skipped_reloads,
                "pending_reloads": self.pending_reloads,
                "last_reload_at": self.last_reload_time
            }

RELOADER = NginxReloader()

CERTBOT_QUEUE = queue.Queue()
# Map primary domain -> {"status": "none"|"ssl_queued"|"ssl_provisioning"|"ssl_active"|"ssl_failed", "error": "", "issued_at": None, "expires_at": None, "retry_count": 0}
SSL_STATUS_STORE = {}

def certbot_worker():
    """Background daemon worker to process Let's Encrypt certificate issuance without blocking HTTP webhooks."""
    while True:
        task = CERTBOT_QUEUE.get()
        if task is None:
            break

        domain = task["domain"]
        subdomain = task["subdomain"]
        custom_domains = task["custom_domains"]
        internal_ip = task["internal_ip"]
        port = task["port"]
        project_dir = task["project_dir"]
        retry_count = task.get("retry_count", 0)

        SSL_STATUS_STORE[domain] = {
            "status": "ssl_provisioning",
            "error": "",
            "retry_count": retry_count,
            "issued_at": None,
            "expires_at": None
        }

        all_domains_list = [domain] + custom_domains
        all_domains_str = " ".join(all_domains_list)
        logging.info(f"[{subdomain}] Background Let's Encrypt provisioning initiated for: {all_domains_str} (Attempt {retry_count + 1})")

        certbot_args = [
            "certbot", "certonly", "--webroot", "-w", "/var/www/html",
            "--non-interactive", "--agree-tos",
            "-m", SSL_EMAIL, "--cert-name", domain, "--expand"
        ]
        for d in all_domains_list:
            certbot_args.extend(["-d", d])

        # Retry loop to mitigate Let's Encrypt / Certbot global lock contention
        max_lock_retries = 5
        lock_retry_delay = 5.0
        ssl_success = False
        ssl_out = ""

        for attempt in range(max_lock_retries):
            ssl_success, ssl_out = run_command(certbot_args)
            if ssl_success:
                break

            # Check if the failure is due to global lock contention
            is_lock_conflict = any(msg in ssl_out for msg in [
                "Another instance of Certbot is already running",
                "lock file",
                "Could not bind",
                "already running"
            ])

            if is_lock_conflict and attempt < max_lock_retries - 1:
                logging.warning(
                    f"[{subdomain}] Certbot lock contention detected. "
                    f"Retrying in {lock_retry_delay}s... (Attempt {attempt + 1}/{max_lock_retries})"
                )
                time.sleep(lock_retry_delay)
            else:
                break

        if ssl_success:
            logging.info(f"[{subdomain}] Background SSL certificates successfully provisioned. Committing Nginx SSL config.")
            with GLOBAL_SYNC_LOCK:
                success, msg, conf_hash = _apply_config_internal(subdomain, domain, all_domains_str, internal_ip, port, project_dir, ssl_enabled=True)
                if success:
                    SSL_STATUS_STORE[domain] = {
                        "status": "ssl_active",
                        "error": "",
                        "retry_count": 0,
                        "issued_at": None,
                        "expires_at": None
                    }
                else:
                    SSL_STATUS_STORE[domain] = {
                        "status": "ssl_failed",
                        "error": f"Nginx commit failed after SSL issuance: {msg}",
                        "retry_count": retry_count + 1,
                        "issued_at": None,
                        "expires_at": None
                    }
        else:
            logging.warning(f"[{subdomain}] Background SSL provisioning failed (Attempt {retry_count + 1}). Error: {ssl_out}")
            SSL_STATUS_STORE[domain] = {
                "status": "ssl_failed",
                "error": ssl_out,
                "retry_count": retry_count + 1,
                "issued_at": None,
                "expires_at": None
            }

        CERTBOT_QUEUE.task_done()

threading.Thread(target=certbot_worker, daemon=True).start()

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
        proxy_hide_header X-Powered-By;

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

def get_file_sha256(filepath):
    """Computes SHA-256 hash of a file for idempotency checking."""
    if not os.path.exists(filepath):
        return None
    h = hashlib.sha256()
    with open(filepath, "rb") as f:
        h.update(f.read())
    return h.hexdigest()

def get_nginx_config(all_domains_str, internal_ip, port, ssl_enabled=False, primary_domain=None):
    """Generates the full Nginx configuration string."""
    proxy_config = PROXY_DIRECTIVES_TEMPLATE.format(
        internal_ip=internal_ip,
        port=port
    )

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

    location /.well-known/acme-challenge/ {{
        root /var/www/html;
    }}

    location / {{
        return 301 https://$host$request_uri;
    }}
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

def cert_covers_all(cert_name, domains):
    """Checks if an existing Let's Encrypt certificate covers all required domains."""
    cert_file = f"/etc/letsencrypt/live/{cert_name}/fullchain.pem"
    if not os.path.exists(cert_file):
        return False
    try:
        cmd = ["openssl", "x509", "-in", cert_file, "-text", "-noout"]
        result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)
        cert_text = result.stdout.lower()
        for d in domains:
            if f"dns:{d.lower()}" not in cert_text:
                return False
        return True
    except Exception as e:
        logging.error(f"Error inspecting certificate {cert_name}: {str(e)}")
        return False

INSPECT_CACHE = {}

def is_valid_hostname(value):
    """Rejects path traversal and malformed certificate/domain identifiers."""
    if not isinstance(value, str) or len(value) > 253:
        return False
    labels = value.rstrip(".").split(".")
    return all(re.fullmatch(r"[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?", label) for label in labels)

def inspect_certificate(cert_name):
    """Returns certificate text and validity dates for a Let's Encrypt cert name with caching."""
    if not is_valid_hostname(cert_name):
        return None
    cert_file = f"/etc/letsencrypt/live/{cert_name}/fullchain.pem"
    if not os.path.exists(cert_file):
        return None
    try:
        stat = os.stat(cert_file)
        cached = INSPECT_CACHE.get(cert_name)
        if cached and cached["mtime"] == stat.st_mtime and cached["size"] == stat.st_size:
            return cached["data"]

        text_cmd = ["openssl", "x509", "-in", cert_file, "-text", "-noout"]
        text_res = subprocess.run(text_cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)
        if text_res.returncode != 0:
            logging.warning(f"Failed to inspect certificate {cert_name}: {text_res.stderr.strip()}")
            return None

        dates_cmd = ["openssl", "x509", "-in", cert_file, "-enddate", "-startdate", "-noout"]
        dates_res = subprocess.run(dates_cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)
        issued_at = None
        expires_at = None
        if dates_res.returncode == 0:
            for line in dates_res.stdout.splitlines():
                if line.startswith("notAfter="):
                    raw_val = line.split("=", 1)[1]
                    expires_at = format_openssl_date(raw_val)
                if line.startswith("notBefore="):
                    raw_val = line.split("=", 1)[1]
                    issued_at = format_openssl_date(raw_val)

        data = {
            "cert_name": cert_name,
            "text": text_res.stdout.lower(),
            "domains": {name.lower().rstrip(".") for name in re.findall(r"DNS:([^,\s]+)", text_res.stdout, re.IGNORECASE)},
            "issued_at": issued_at,
            "expires_at": expires_at,
        }
        INSPECT_CACHE[cert_name] = {
            "mtime": stat.st_mtime,
            "size": stat.st_size,
            "data": data
        }
        return data
    except Exception as e:
        logging.error(f"Error inspecting certificate {cert_name}: {str(e)}")
        return None

def certificate_covers_domain(cert_info, domain):
    """Checks exact SAN ownership; unrelated certificate lineages must not satisfy project SSL."""
    return bool(cert_info and domain.lower().rstrip(".") in cert_info["domains"])

def _apply_config_internal(subdomain, domain, all_domains_str, internal_ip, port, project_dir, ssl_enabled):
    """Stages configuration atomically, verifies syntax, and schedules debounced reload."""
    file_path = os.path.join(project_dir, f"project-{subdomain}.conf")
    temp_path = f"{file_path}.tmp"
    backup_path = f"{file_path}.bak"

    conf_content = get_nginx_config(all_domains_str, internal_ip, port, ssl_enabled=ssl_enabled, primary_domain=domain)
    new_hash = hashlib.sha256(conf_content.encode("utf-8")).hexdigest()

    old_hash = get_file_sha256(file_path)
    if old_hash == new_hash:
        logging.info(f"[{subdomain}] Nginx configuration hash ({new_hash[:8]}) matches active config. Skipping rewrite and reload.")
        return True, "Synced (Hash Match)", new_hash

    with open(temp_path, "w") as f:
        f.write(conf_content)

    old_existed = os.path.exists(file_path)
    if old_existed:
        os.rename(file_path, backup_path)
    os.rename(temp_path, file_path)

    test_success, test_out = run_command(["nginx", "-t"])
    if not test_success:
        logging.error(f"[{subdomain}] Nginx syntax validation failed. Rolling back configuration.")
        os.remove(file_path)
        if old_existed:
            os.rename(backup_path, file_path)
        return False, f"Nginx syntax error: {test_out}", None

    if os.path.exists(backup_path):
        os.remove(backup_path)

    RELOADER.schedule_reload()
    return True, "Synced", new_hash

def sync_project(subdomain, domain, custom_domains, internal_ip, port, project_dir):
    """Handles project Nginx configuration using an Atomic Commit workflow with Smart SSL."""
    all_domains_list = [domain] + custom_domains
    all_domains_str = " ".join(all_domains_list)

    needs_ssl_expansion = not cert_covers_all(domain, all_domains_list)
    has_ssl_options = os.path.exists("/etc/letsencrypt/options-ssl-nginx.conf")

    # Avoid dropping HTTPS for existing working domains if Let's Encrypt expansion is queued or fails
    primary_cert_exists = os.path.exists(f"/etc/letsencrypt/live/{domain}/fullchain.pem")
    use_ssl = (primary_cert_exists or not needs_ssl_expansion) and has_ssl_options

    success, msg, conf_hash = _apply_config_internal(subdomain, domain, all_domains_str, internal_ip, port, project_dir, ssl_enabled=use_ssl)
    if not success:
        return False, msg, None

    if needs_ssl_expansion:
        current_status = SSL_STATUS_STORE.get(domain, {}).get("status")
        if current_status not in ["ssl_queued", "ssl_provisioning"]:
            retry_count = SSL_STATUS_STORE.get(domain, {}).get("retry_count", 0)
            SSL_STATUS_STORE[domain] = {
                "status": "ssl_queued",
                "error": "",
                "retry_count": retry_count,
                "issued_at": None,
                "expires_at": None
            }
            CERTBOT_QUEUE.put({
                "domain": domain,
                "subdomain": subdomain,
                "custom_domains": custom_domains,
                "internal_ip": internal_ip,
                "port": port,
                "project_dir": project_dir,
                "retry_count": retry_count
            })
            logging.info(f"[{subdomain}] Let's Encrypt expansion enqueued for asynchronous background issuance.")
        return True, "Synced (SSL Queued)", conf_hash

    return True, "Synced (SSL Active)", conf_hash

def delete_project(subdomain, project_dir):
    """Cleans up project Nginx configuration."""
    file_path = os.path.join(project_dir, f"project-{subdomain}.conf")
    if not os.path.exists(file_path):
        return True, "Not found"
    os.remove(file_path)
    RELOADER.schedule_reload()
    logging.info(f"[{subdomain}] Deleted configuration")
    return True, "Deleted"

SEEN_NONCES = {}

def verify_webhook_signature(req):
    """Verifies HMAC SHA-256 signature, timestamp expiration (5m window), and prevents replay attacks via nonce caching."""
    timestamp_str = req.headers.get("X-Webhook-Timestamp")
    nonce = req.headers.get("X-Webhook-Nonce")
    signature = req.headers.get("X-Webhook-Signature")
    legacy_key = req.headers.get("X-Webhook-Key")

    if not timestamp_str or not nonce or not signature:
        if legacy_key == WEBHOOK_KEY:
            return True, "Valid (Legacy)"
        return False, "Missing signature headers"

    try:
        timestamp = int(timestamp_str)
    except (ValueError, TypeError):
        return False, "Invalid timestamp"

    now = int(time.time())
    if abs(now - timestamp) > 300:
        return False, f"Request expired (timestamp difference: {abs(now - timestamp)}s)"

    # Replay attack prevention
    stale = [n for n, ts in list(SEEN_NONCES.items()) if abs(now - ts) > 300]
    for n in stale:
        del SEEN_NONCES[n]

    if nonce in SEEN_NONCES:
        return False, "Replay attack detected (reused nonce)"

    path = req.path
    if req.query_string:
        path = f"{path}?{req.query_string.decode('utf-8')}"

    body = req.get_data(as_text=True)
    raw_str = f"{timestamp_str}:{nonce}:{req.method}:{path}:{body}"

    expected_sig = hmac.new(WEBHOOK_KEY.encode('utf-8'), raw_str.encode('utf-8'), hashlib.sha256).hexdigest()
    if not hmac.compare_digest(signature, expected_sig):
        return False, "Signature mismatch"

    SEEN_NONCES[nonce] = now
    return True, "Valid"

@app.route('/webhook', methods=['POST'])
def webhook():
    """Main webhook entry point."""
    client_ip = request.remote_addr
    valid, err_msg = verify_webhook_signature(request)

    if not valid:
        logging.warning(f"Unauthorized access from {client_ip}: {err_msg}")
        return jsonify({"error": f"Unauthorized: {err_msg}"}), 401

    data = request.get_json(force=True, silent=True) or {}
    action = data.get("action")
    subdomain = data.get("subdomain")
    domain = data.get("domain")
    custom_domains = data.get("custom_domains", [])
    user_folder = data.get("user_folder", "project-user")

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

            success, message, conf_hash = sync_project(subdomain, domain, custom_domains, internal_ip, port, project_dir)
            return jsonify({"message": message, "config_hash": conf_hash}), 200 if success else 500

        if action == "delete":
            success, message = delete_project(subdomain, project_dir)
            return jsonify({"message": message}), 200

    return jsonify({"error": "Invalid action"}), 400

@app.route('/ssl-status', methods=['GET'])
def ssl_status():
    """Returns Let's Encrypt issuance status and OpenSSL certificate valid dates."""
    client_ip = request.remote_addr
    valid, err_msg = verify_webhook_signature(request)
    if not valid:
        logging.warning(f"Unauthorized ssl-status access from {client_ip}: {err_msg}")
        return jsonify({"error": f"Unauthorized: {err_msg}"}), 401

    cert_name = request.args.get("cert_name")
    domain = request.args.get("domain")
    if not cert_name or not domain:
        return jsonify({"error": "Missing cert_name or domain parameter"}), 400
    if not is_valid_hostname(cert_name) or not is_valid_hostname(domain):
        return jsonify({"error": "Invalid cert_name or domain parameter"}), 400

    status_info = SSL_STATUS_STORE.get(cert_name, {})
    current_status = status_info.get("status", "none")
    error_msg = status_info.get("error", "")
    retry_count = status_info.get("retry_count", 0)

    expires_at = None
    issued_at = None
    cert_info = inspect_certificate(cert_name)
    if certificate_covers_domain(cert_info, domain):
        issued_at = cert_info["issued_at"]
        expires_at = cert_info["expires_at"]
        current_status = "ssl_active"
        error_msg = ""
        logging.info(f"SSL status active for {domain}; covered by project certificate {cert_name}")
    elif current_status == "ssl_active":
        current_status = "ssl_failed"
        error_msg = f"Project certificate {cert_name} does not cover {domain}"

    return jsonify({
        "cert_name": cert_name,
        "domain": domain,
        "status": current_status,
        "error": error_msg,
        "issued_at": issued_at,
        "expires_at": expires_at,
        "retry_count": retry_count
    }), 200

@app.route('/nginx-metrics', methods=['GET'])
def nginx_metrics():
    """Returns Nginx reload coalescing and execution metrics."""
    client_ip = request.remote_addr
    valid, err_msg = verify_webhook_signature(request)
    if not valid:
        logging.warning(f"Unauthorized nginx-metrics access from {client_ip}: {err_msg}")
        return jsonify({"error": f"Unauthorized: {err_msg}"}), 401

    return jsonify(RELOADER.get_metrics()), 200

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=LISTEN_PORT)
