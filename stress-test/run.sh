#!/bin/bash

# Pastikan script ini dijalankan dari root project
# Usage: ./stress-test/run.sh [URL_TARGET]

TARGET_URL=${1:-"http://localhost:8080"}

echo "--------------------------------------------------------"
echo "🚀 Starting Stress Test for: $TARGET_URL"
echo "🔧 Tool: k6 (Running via Docker)"
echo "🖥️  Spec: 8 Core, 8GB RAM"
echo "--------------------------------------------------------"

# Run k6 using Docker
docker run --rm -i \
  -e TARGET_URL=$TARGET_URL \
  grafana/k6 run - <stress-test/k6-script.js

echo "--------------------------------------------------------"
echo "✅ Test Completed."
echo "Lihat 'http_req_duration' untuk performa latency (P95)."
echo "Lihat 'http_req_failed' untuk kestabilan error rate."
echo "--------------------------------------------------------"
