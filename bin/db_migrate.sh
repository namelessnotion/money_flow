#!/bin/bash
set -euo pipefail

DATABASE_URL="${DATABASE_URL:-postgres://money_flow:money_flow@localhost:5432/money_flow_dev?sslmode=disable}"
export DATABASE_URL

cd "$(dirname "$0")/../go"
go run ./cmd/migrate "${1:-up}"
