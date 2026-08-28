#!/usr/bin/env bash
# Source hypervisor SSH passwords from infra/docker/.env.probe (mode 600).
set -euo pipefail

load_ops_env() {
  local root="${1:-.}"
  local env_probe="${ENV_PROBE:-$root/infra/docker/.env.probe}"
  if [[ ! -f "$env_probe" ]]; then
    echo "Missing $env_probe — copy from .env.probe.example and set NL/DE/FI/GB passwords" >&2
    exit 1
  fi
  # shellcheck disable=SC1090
  set -a && source "$env_probe" && set +a
}

ops_ssh_opts() {
  local root="${1:-.}"
  local known_hosts="${OPS_KNOWN_HOSTS:-$root/infra/docker/ops-known_hosts}"
  if [[ -f "$known_hosts" ]]; then
    echo "-o StrictHostKeyChecking=yes -o UserKnownHostsFile=${known_hosts}"
  else
    echo "-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=${known_hosts}"
  fi
}

ssh_hv() {
  local ip="$1" pass="$2"
  shift 2
  local root="${OPS_ROOT:-.}"
  # shellcheck disable=SC2046
  SSHPASS="$pass" sshpass -e ssh $(ops_ssh_opts "$root") -o ConnectTimeout=20 "root@${ip}" "$@"
}

scp_hv() {
  local ip="$1" pass="$2" src="$3" dst="$4"
  local root="${OPS_ROOT:-.}"
  # shellcheck disable=SC2046
  SSHPASS="$pass" sshpass -e scp $(ops_ssh_opts "$root") -o ConnectTimeout=20 "$src" "root@${ip}:${dst}"
}
