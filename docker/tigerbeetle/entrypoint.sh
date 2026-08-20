#!/bin/sh
set -eu

data_file="${TIGERBEETLE_DATA_FILE:-/data/0_0.tigerbeetle}"

if [ ! -f "$data_file" ]; then
  /tigerbeetle format \
    --cluster="${TIGERBEETLE_CLUSTER:-0}" \
    --replica="${TIGERBEETLE_REPLICA:-0}" \
    --replica-count="${TIGERBEETLE_REPLICA_COUNT:-1}" \
    --development \
    "$data_file"
fi

exec /tigerbeetle start \
  --addresses="${TIGERBEETLE_ADDRESSES:-0.0.0.0:3000}" \
  --development \
  "$data_file"
