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
sed -e "s|paas-backend:8080|127.0.0.1:$port|" -e 's|address: ":80"|address: ":18080"|' \
  "$root/docker/traefik/traefik.yml.template" > "$tmp/traefik.yml.template"
TRAEFIK_PRIVATE_DIR=$private_dir
INTERNAL_API_TOKEN=$token
config=$(render_traefik_static_config "$tmp/traefik.yml.template")
[ "$config" = "$private_dir/traefik.yml" ]
[ "$(stat -c '%a:%u' "$private_dir")" = "700:$(id -u)" ]
[ "$(stat -c '%a:%u' "$config")" = "600:$(id -u)" ]
[ "$(realpath "$config")" != "$(realpath "$tmp/dynamic")/traefik.yml" ]

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
        with open(output, "w", encoding="utf-8") as f:
            f.write(self.headers.get("X-Internal-Token", ""))
        self.send_response(200 if self.headers.get("X-Internal-Token") == token else 403)
        self.end_headers()
        self.wfile.write(b"http: {}\n")
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
