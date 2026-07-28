#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf '%s\n' "$1" >&2
  return 1
}

check_policy() {
  local root=$1 script block

  for script in start.sh deploy-app.sh stop.sh infra.sh; do
    [ -f "$root/scripts/$script" ] || fail "missing policy target: $script" || return 1
  done
  [ -f "$root/.dockerignore" ] || fail 'missing .dockerignore' || return 1
  [ -f "$root/docker/traefik/traefik.yml.template" ] || fail 'missing Traefik static template' || return 1
  [ -f "$root/docker/traefik/dynamic.yml.template" ] || fail 'missing Traefik dynamic template' || return 1

  for script in start.sh deploy-app.sh stop.sh; do
    block=$(sed -n '/^start_worker()\|^deploy_worker()/,/^}/p' "$root/scripts/$script")
    if grep -Eq '(/\.env:|JWT_|CSRF_|BILLING_|MIDTRANS_|INTERNAL_API_TOKEN)' <<<"$block"; then
      fail "worker secret boundary failed: $script" || return 1
    fi
  done

  grep -qx '.env' "$root/.dockerignore" || fail '.env must stay excluded from Docker context' || return 1
  grep -q '__INTERNAL_API_TOKEN__' "$root/docker/traefik/traefik.yml.template" || fail 'Traefik static template must retain token placeholder' || return 1

  for script in start.sh stop.sh infra.sh; do
    if ! grep -q 'traefik-config.sh' "$root/scripts/$script"; then
      fail "Traefik helper missing: $script" || return 1
    fi
    if ! grep -q 'render_traefik_static_config' "$root/scripts/$script"; then
      fail "Traefik renderer missing: $script" || return 1
    fi
    if ! grep -Fq 'chmod 700 "$TRAEFIK_DYNAMIC_DIR"' "$root/scripts/$script"; then
      fail "Traefik dynamic directory must be mode 700: $script" || return 1
    fi
    if grep -Eq 'TRAEFIK_DYNAMIC_DIR.*/traefik\.yml' "$root/scripts/$script"; then
      fail "static Traefik config in dynamic directory: $script" || return 1
    fi
    if grep -Eq 'chmod[[:space:]]+777[[:space:]].*TRAEFIK_DYNAMIC_DIR' "$root/scripts/$script"; then
      fail "world-writable Traefik dynamic directory: $script" || return 1
    fi
    if ! grep -q ':/etc/traefik/dynamic:ro' "$root/scripts/$script"; then
      fail "Traefik dynamic mount must be read-only: $script" || return 1
    fi
  done

  if grep -q 'accessControlAllowOriginList' "$root/docker/traefik/dynamic.yml.template"; then
    fail 'wildcard CORS policy is forbidden' || return 1
  fi
  if grep -Fq 'Host(`*.{{BASE_DOMAIN}}`)' "$root/docker/traefik/dynamic.yml.template"; then
    fail 'wildcard Traefik host rule is forbidden' || return 1
  fi
}

self_test() {
  local root=$1 tmp fixture case path content
  tmp=$(mktemp -d)
  fixture="$tmp/fixture"
  mkdir -p "$fixture/scripts" "$fixture/docker/traefik"
  cp "$root/.dockerignore" "$fixture/.dockerignore"
  cp "$root/scripts/start.sh" "$root/scripts/stop.sh" "$root/scripts/infra.sh" "$root/scripts/deploy-app.sh" "$fixture/scripts/"
  cp "$root/docker/traefik/traefik.yml.template" "$root/docker/traefik/dynamic.yml.template" "$fixture/docker/traefik/"
  check_policy "$fixture"

  while IFS='|' read -r path content; do
    case="$tmp/case-${path//\//-}-${content:0:8}"
    cp -R "$fixture" "$case"
    printf '\n%s\n' "$content" >> "$case/$path"
    if check_policy "$case"; then
      fail "policy mutation passed: $content"
      return 1
    fi
  done <<'MUTATIONS'
scripts/start.sh|TRAEFIK_CONFIG="$TRAEFIK_DYNAMIC_DIR/traefik.yml"
scripts/stop.sh|chmod 777 "$TRAEFIK_DYNAMIC_DIR"
docker/traefik/dynamic.yml.template|accessControlAllowOriginList: ["*"]
docker/traefik/dynamic.yml.template|Host(`*.{{BASE_DOMAIN}}`)
MUTATIONS
  rm -rf "$tmp"
}

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
check_policy "$root"
self_test "$root"
"$root/scripts/check-traefik-provider.sh"
