#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
service_name=runara-traefik.service
unit_path="/etc/systemd/system/$service_name"
deployment_uid=${SUDO_UID:-$(id -u)}
deployment_gid=${SUDO_GID:-$(id -g)}

if [[ ! $root =~ ^/[A-Za-z0-9._/-]+$ ]]; then
  printf 'Project path contains unsupported characters: %s\n' "$root" >&2
  exit 1
fi

render_unit() {
  cat <<EOF
[Unit]
Description=Runara Traefik boot recovery
Requires=docker.service
After=docker.service network-online.target
Wants=network-online.target
PartOf=docker.service

[Service]
Type=oneshot
WorkingDirectory=$root
Environment=APP_UID=$deployment_uid
Environment=APP_GID=$deployment_gid
ExecStart=$root/scripts/start.sh traefik
RemainAfterExit=yes
Restart=on-failure
RestartSec=10
TimeoutStartSec=180
UMask=0077

[Install]
WantedBy=multi-user.target
EOF
}

if [ "${1:-}" = "--dry-run" ]; then
  render_unit
  exit 0
fi

if [ "$#" -ne 0 ]; then
  printf 'Usage: %s [--dry-run]\n' "$0" >&2
  exit 1
fi

command -v systemctl >/dev/null 2>&1 || { printf 'systemctl is required\n' >&2; exit 1; }
command -v docker >/dev/null 2>&1 || { printf 'docker is required\n' >&2; exit 1; }

privileged=()
if [ "$(id -u)" -ne 0 ]; then
  privileged=(sudo)
fi

render_unit | "${privileged[@]}" tee "$unit_path" >/dev/null
"${privileged[@]}" chmod 644 "$unit_path"
"${privileged[@]}" systemctl daemon-reload
"${privileged[@]}" systemctl enable --now "$service_name"
"${privileged[@]}" systemctl --no-pager --full status "$service_name"
