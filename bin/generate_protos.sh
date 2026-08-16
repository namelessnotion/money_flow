#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")/.."

TOOLS_DIR="./go/.tools"
mkdir -p "$TOOLS_DIR"

(cd go && go build -o ".tools/protoc-gen-go" google.golang.org/protobuf/cmd/protoc-gen-go)
(cd go && go build -o ".tools/protoc-gen-twirp" github.com/twitchtv/twirp/protoc-gen-twirp)
(cd go && go build -o ".tools/protoc-gen-twirp_ruby" github.com/arthurnn/twirp-ruby/protoc-gen-twirp_ruby)

PROTO_FILES=()
while IFS= read -r f; do
  PROTO_FILES+=("$f")
done < <(find proto -name '*.proto' | sort)

rm -rf go/gen/proto
rm -rf ruby/gen/proto
mkdir -p go/gen/proto ruby/gen/proto

protoc --proto_path=./proto \
  --plugin=protoc-gen-go="$TOOLS_DIR/protoc-gen-go" --go_out=./go/gen/proto --go_opt=paths=source_relative \
  "${PROTO_FILES[@]}"

protoc --proto_path=./proto \
  --ruby_out=./ruby/gen/proto \
  "${PROTO_FILES[@]}"

for f in "${PROTO_FILES[@]}"; do
  grep -q '^service ' "$f" || continue

  protoc --proto_path=./proto \
    --plugin=protoc-gen-twirp="$TOOLS_DIR/protoc-gen-twirp" --twirp_out=./go/gen/proto --twirp_opt=paths=source_relative \
    "$f"

  protoc --proto_path=./proto \
    --plugin=protoc-gen-twirp_ruby="$TOOLS_DIR/protoc-gen-twirp_ruby" --twirp_ruby_out=./ruby/gen/proto \
    "$f"
done
