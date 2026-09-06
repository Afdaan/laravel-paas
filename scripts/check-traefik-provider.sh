#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source "$root/scripts/traefik-config.sh"
tmp=$(mktemp -d)
private_dir="$tmp/private"
invalid_private_dir="${private_dir}-invalid"
trap 'rm -rf "$tmp"; kill "${server_pid:-}" "${traefik_pid:-}" 2>/dev/null || true' EXIT
port=18081
token=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef

mkdir -m 700 "$tmp/dynamic"
sed -e "s|paas-backend:8080|127.0.0.1:$port|" \
  -e 's|address: ":80"|address: ":18080"|' \
  -e 's|address: ":443"|address: ":18443"|' \
  "$root/docker/traefik/traefik.yml.template" > "$tmp/traefik.yml.template"
sed -e 's/{{BASE_DOMAIN}}/platform.test/g' \
  -e 's/{{PROJECT_DOMAIN}}/projects.test/g' \
  -e "s|http://paas-backend:8080/proxy/|http://127.0.0.1:$port/proxy/|" \
  -e "s|http://paas-backend:8080|http://127.0.0.1:$port/api/|" \
  -e "s|http://paas-frontend:80|http://127.0.0.1:$port/frontend/|" \
  "$root/docker/traefik/dynamic.yml.template" > "$tmp/dynamic/dynamic.yml"
TRAEFIK_PRIVATE_DIR=$private_dir
INTERNAL_API_TOKEN=$token
config=$(render_traefik_static_config "$tmp/traefik.yml.template")
[ "$config" = "$private_dir/traefik.yml" ]
[ "$(stat -c '%a:%u' "$private_dir")" = "700:$(id -u)" ]
[ "$(stat -c '%a:%u' "$config")" = "600:$(id -u)" ]
[ "$(realpath "$config")" != "$(realpath "$tmp/dynamic")/traefik.yml" ]

# Verify rendered config does NOT auto-trust Docker bridge gateways
! grep -E '172\.(17|18|19|20|28|29)\.0\.1/32' "$config"

# Verify invalid, wildcard, and broad CIDR values are strictly rejected
for bad_cidr in '1.2.3.4/0' '0:0:0:0:0:0:0:0/0' '::::/128' '172.28.0.0/16' '10.0.0.0/8' 'fc00::/7' '999.1.1.1' '0.0.0.0'; do
  if (TRAEFIK_PRIVATE_DIR=$invalid_private_dir INTERNAL_API_TOKEN="$token" TRAEFIK_TRUSTED_IPS="$bad_cidr" render_traefik_static_config "$tmp/traefik.yml.template") >/dev/null 2>&1; then
    printf 'insecure/invalid CIDR accepted: %s\n' "$bad_cidr" >&2
    exit 1
  fi
done

# Verify valid exact peer CIDR normalization
valid_test_dir="$tmp/valid_test"
mkdir -m 700 "$valid_test_dir"
valid_config=$(TRAEFIK_PRIVATE_DIR="$valid_test_dir" INTERNAL_API_TOKEN="$token" TRAEFIK_TRUSTED_IPS="172.29.0.4, 2001:db8::1" render_traefik_static_config "$tmp/traefik.yml.template")
grep -F '172.29.0.4/32' "$valid_config"
grep -F '2001:db8::1/128' "$valid_config"

for invalid in '"' '\\' $'\r' $'\n' ' ' g; do
  if (TRAEFIK_PRIVATE_DIR=$invalid_private_dir INTERNAL_API_TOKEN="${token:0:63}$invalid" render_traefik_static_config "$tmp/traefik.yml.template") >/dev/null 2>&1; then
    printf 'unsafe token accepted\n' >&2
    exit 1
  fi
done
[ ! -e "$invalid_private_dir/traefik.yml" ]

python3 - "$port" "$token" "$tmp/header" <<'PY' &
import http.server
import sys

port, token, output = int(sys.argv[1]), sys.argv[2], sys.argv[3]
class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path.startswith("/api/internal/traefik/config"):
            with open(output, "w", encoding="utf-8") as f:
                f.write(self.headers.get("X-Internal-Token", ""))
            self.send_response(200 if self.headers.get("X-Internal-Token") == token else 403)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"http": {}}')
            return

        self.send_response(200)
        self.send_header("Permissions-Policy", "camera=(self)")
        self.send_header("X-Powered-By", "test-backend")
        self.end_headers()
        self.wfile.write(b"ok")
    def log_message(self, *_):
        pass
http.server.HTTPServer(("127.0.0.1", port), Handler).serve_forever()
PY
server_pid=$!

docker run --rm --network host \
  -v "$config:/traefik.yml:ro" \
  -v "$tmp/dynamic:/etc/traefik/dynamic:ro" \
  traefik:v3.6 --configFile=/traefik.yml > "$tmp/traefik.log" 2>&1 &
traefik_pid=$!

for _ in $(seq 1 20); do
  [ -f "$tmp/header" ] && break
  sleep 0.5
done

grep -qx "$token" "$tmp/header"
grep -F 'Starting provider *http.Provider' "$tmp/traefik.log"

for _ in $(seq 1 20); do
  if curl -fsS -D "$tmp/project.headers" -o /dev/null -H 'Host: demo.projects.test' http://127.0.0.1:18080/ 2>/dev/null; then
    break
  fi
  sleep 0.5
done

grep -Eiq '^Permissions-Policy: camera=\(self\)' "$tmp/project.headers"
! grep -Eiq '^Permissions-Policy: .*camera=\(\)' "$tmp/project.headers"
! grep -Eiq '^X-Powered-By:' "$tmp/project.headers"
grep -Eiq '^X-Content-Type-Options: nosniff' "$tmp/project.headers"

curl -fsS -D "$tmp/platform.headers" -o /dev/null -H 'Host: platform.test' http://127.0.0.1:18080/
grep -Eiq '^Permissions-Policy: geolocation=\(\), microphone=\(\), camera=\(\)' "$tmp/platform.headers"
grep -Eiq '^X-Frame-Options: DENY' "$tmp/platform.headers"
