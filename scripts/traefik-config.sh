#!/usr/bin/env bash

valid_internal_api_token() {
  [[ ${#INTERNAL_API_TOKEN} -eq 64 && $INTERNAL_API_TOKEN =~ ^[0-9a-f]{64}$ ]]
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
  if ! printf '%s' "${config//__INTERNAL_API_TOKEN__/$INTERNAL_API_TOKEN}" | "${privileged[@]}" tee "$tmp" >/dev/null; then
    "${privileged[@]}" rm -f -- "$tmp"
    return 1
  fi
  "${privileged[@]}" chmod 600 "$tmp"
  "${privileged[@]}" mv -f -- "$tmp" "$private_dir/traefik.yml"
  printf '%s\n' "$private_dir/traefik.yml"
}
