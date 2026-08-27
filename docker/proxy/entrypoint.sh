#!/bin/sh
# Generates a throwaway self-signed cert on first boot if bin/setup_local_ssl.sh
# hasn't produced a real (mkcert-trusted) one yet, so `docker compose up`
# always succeeds over HTTPS — browsers just warn until the real cert exists
# at the same path. Mirrors the format-if-needed pattern in
# docker/tigerbeetle/entrypoint.sh.
set -eu

cert_dir=/etc/nginx/certs
cert="$cert_dir/local.namelessnotion.com.pem"
key="$cert_dir/local.namelessnotion.com-key.pem"

if [ ! -f "$cert" ] || [ ! -f "$key" ]; then
  echo "entrypoint: no cert at $cert, generating a self-signed placeholder (run bin/setup_local_ssl.sh on the host for a trusted one)"
  openssl req -x509 -nodes -newkey rsa:2048 -days 365 \
    -keyout "$key" -out "$cert" \
    -subj "/CN=local.namelessnotion.com" \
    -addext "subjectAltName=DNS:local.namelessnotion.com,DNS:*.local.namelessnotion.com"
fi

exec "$@"
