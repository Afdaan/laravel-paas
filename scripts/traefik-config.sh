#!/usr/bin/env bash

valid_internal_api_token() {
  [[ ${#INTERNAL_API_TOKEN} -eq 64 && $INTERNAL_API_TOKEN =~ ^[0-9a-f]{64}$ ]]
}

validate_and_normalize_trusted_cidr() {
  local entry=$1
  if ! command -v python3 >/dev/null 2>&1; then
    printf 'python3 is required for IP/CIDR validation\n' >&2
    return 1
  fi

  python3 -c '
import sys, ipaddress

raw = sys.argv[1].strip()
if not raw:
    sys.exit(0)

try:
    if "/" not in raw:
        ip = ipaddress.ip_address(raw)
        net = ipaddress.ip_network(f"{ip}/{ip.max_prefixlen}")
    else:
        net = ipaddress.ip_network(raw, strict=False)
except ValueError as e:
    sys.stderr.write(f"Invalid IP/CIDR syntax: {raw} ({e})\n")
    sys.exit(1)

# Reject wildcards and unspecified networks (e.g. 0.0.0.0/0, ::/0, 1.2.3.4/0, 0:0:0:0:0:0:0:0/0)
if net.prefixlen == 0 or net.is_unspecified:
    sys.stderr.write(f"Cannot trust wildcard/unspecified network: {raw}\n")
    sys.exit(1)

# Enforce exact peer host (/32 or /128) for private, loopback, link-local, or reserved networks
if net.is_private or net.is_loopback or net.is_link_local or net.is_reserved:
    if net.prefixlen != net.max_prefixlen:
        sys.stderr.write(f"TRAEFIK_TRUSTED_IPS private/local range must be exact /{net.max_prefixlen}, got broad subnet: {raw}\n")
        sys.exit(1)

print(str(net))
' "$entry"
}

build_traefik_trusted_ips_yaml() {
  local list=("127.0.0.1/32" "::1/128")

  # User-configured TRAEFIK_TRUSTED_IPS (e.g. exact containerized tunnel or custom proxy /32 or /128)
  if [ -n "${TRAEFIK_TRUSTED_IPS:-}" ]; then
    IFS=',' read -ra user_ips <<< "$TRAEFIK_TRUSTED_IPS"
    for ip in "${user_ips[@]}"; do
      local trimmed_ip=$(echo "$ip" | tr -d '[:space:]')
      if [ -n "$trimmed_ip" ]; then
        local valid_cidr
        if ! valid_cidr=$(validate_and_normalize_trusted_cidr "$trimmed_ip"); then
          return 1
        fi
        list+=("$valid_cidr")
      fi
    done
  fi

  # Official Cloudflare IPv4 CIDRs
  list+=(
    "173.245.48.0/20"
    "103.21.244.0/22"
    "103.22.200.0/22"
    "103.31.4.0/22"
    "141.101.64.0/18"
    "108.162.192.0/18"
    "190.93.240.0/20"
    "188.114.96.0/20"
    "197.234.240.0/22"
    "198.41.128.0/17"
    "162.158.0.0/15"
    "104.16.0.0/13"
    "104.24.0.0/14"
    "172.64.0.0/13"
    "131.0.72.0/22"
  )

  # Official Cloudflare IPv6 CIDRs
  list+=(
    "2400:cb00::/32"
    "2606:4700::/32"
    "2803:f800::/32"
    "2405:b500::/32"
    "2405:8100::/32"
    "2a06:98c0::/29"
    "2c0f:f248::/32"
  )

  # Deduplicate and format each line with 8 spaces indentation
  printf '%s\n' "${list[@]}" | awk 'NF && !seen[$0]++ { printf "        - \"%s\"\n", $0 }'
}

render_traefik_static_config() {
  local template=$1 private_dir=${TRAEFIK_PRIVATE_DIR:-/run/runara} private_parent tmp stat config owner group
  local -a privileged=()

  valid_internal_api_token || { printf 'INTERNAL_API_TOKEN must be 64 lowercase hexadecimal characters\n' >&2; return 1; }
  [ -f "$template" ] || { printf 'Traefik template not found: %s\n' "$template" >&2; return 1; }
  if [ -z "${TRAEFIK_PRIVATE_DIR:-}" ]; then
    privileged=(sudo)
    owner=0
    group=0
  else
    owner=$(id -u)
    group=$(id -g)
  fi
  private_parent=$(dirname "$private_dir")
  "${privileged[@]}" test -d "$private_parent" && "${privileged[@]}" test ! -L "$private_parent" || { printf 'Traefik private parent is unsafe\n' >&2; return 1; }
  stat=$("${privileged[@]}" stat -c '%u:%a' "$private_parent")
  [[ ${stat%%:*} == "$owner" ]] && (( (8#${stat##*:} & 0022) == 0 )) || { printf 'Traefik private parent must be owner-only writable\n' >&2; return 1; }
  "${privileged[@]}" test ! -L "$private_dir" || { printf 'Traefik private directory is a symlink\n' >&2; return 1; }
  umask 077
  "${privileged[@]}" install -d -o "$owner" -g "$group" -m 700 "$private_dir"
  [[ $("${privileged[@]}" stat -c '%u:%a' "$private_dir") == "$owner:700" ]] || { printf 'Traefik private directory permissions are unsafe\n' >&2; return 1; }
  tmp=$("${privileged[@]}" mktemp "$private_dir/.traefik.yml.XXXXXX")
  config=$(<"$template")
  local trusted_ips_yaml
  if ! trusted_ips_yaml=$(build_traefik_trusted_ips_yaml); then
    printf 'Failed to build Traefik trusted IPs\n' >&2
    return 1
  fi
  config="${config//__INTERNAL_API_TOKEN__/$INTERNAL_API_TOKEN}"
  config="${config//__TRAEFIK_TRUSTED_IPS__/$trusted_ips_yaml}"
  if ! printf '%s' "$config" | "${privileged[@]}" tee "$tmp" >/dev/null; then
    "${privileged[@]}" rm -f -- "$tmp"
    return 1
  fi
  "${privileged[@]}" chmod 600 "$tmp"
  "${privileged[@]}" mv -f -- "$tmp" "$private_dir/traefik.yml"
  printf '%s\n' "$private_dir/traefik.yml"
}
