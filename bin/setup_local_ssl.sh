#!/bin/bash
# Generates a browser-trusted cert for *.local.namelessnotion.com using mkcert,
# so the proxy container can serve https://app.local.namelessnotion.com (and the
# graphql/rpc subdomains) without certificate warnings.
#
# Until this has been run, the proxy container serves a self-signed
# placeholder instead (see docker/proxy/entrypoint.sh) so `make up` still
# works over HTTPS, just with a browser warning.
set -euo pipefail

CERT_DIR="$(cd "$(dirname "$0")/.." && pwd)/docker/proxy/certs"
mkdir -p "$CERT_DIR"

if ! command -v mkcert >/dev/null 2>&1; then
  cat >&2 <<'EOF'
mkcert is required but wasn't found on PATH.

Install it, then re-run this script:
  macOS:          brew install mkcert
  Debian/Ubuntu:  apt install mkcert
  other:          https://github.com/FiloSottile/mkcert#installation
EOF
  exit 1
fi

# Installs mkcert's local CA into the system and browser trust stores. Safe
# to run repeatedly — it's a no-op once the CA is already installed. May
# prompt for your password (sudo) or keychain access.
mkcert -install

cd "$CERT_DIR"
mkcert -cert-file local.namelessnotion.com.pem -key-file local.namelessnotion.com-key.pem \
  "local.namelessnotion.com" "*.local.namelessnotion.com"

echo
echo "Wrote $CERT_DIR/local.namelessnotion.com.pem and local.namelessnotion.com-key.pem"
echo "Run 'make ssl' next time to also restart the proxy container, or now: docker compose restart proxy"
