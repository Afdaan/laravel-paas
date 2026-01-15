#!/bin/bash
# ===========================================
# Webhook Client - panggil dari PaaS VPS
# ===========================================
# Usage:
#   ./webhook-client.sh create project-abc 192.168.255.114 3001
#   ./webhook-client.sh delete project-abc
# ===========================================

WEBHOOK_URL="${WEBHOOK_URL:-http://192.168.255.1:5000}"
WEBHOOK_SECRET="${WEBHOOK_SECRET:-change-this-secret}"

action="$1"
subdomain="$2"
backend_ip="$3"
port="$4"

case "$action" in
    create|update)
        curl -s -X POST "${WEBHOOK_URL}/webhook/${action}" \
            -H "Content-Type: application/json" \
            -H "X-Webhook-Secret: ${WEBHOOK_SECRET}" \
            -d "{\"subdomain\":\"${subdomain}\",\"backend_ip\":\"${backend_ip}\",\"port\":${port}}"
        ;;
    delete)
        curl -s -X POST "${WEBHOOK_URL}/webhook/delete" \
            -H "Content-Type: application/json" \
            -H "X-Webhook-Secret: ${WEBHOOK_SECRET}" \
            -d "{\"subdomain\":\"${subdomain}\"}"
        ;;
    health)
        curl -s "${WEBHOOK_URL}/webhook/health"
        ;;
    *)
        echo "Usage: $0 <create|update|delete|health> <subdomain> [backend_ip] [port]"
        exit 1
        ;;
esac
