#!/usr/bin/env bash
set -euo pipefail

get_size() {
  if stat --version >/dev/null 2>&1; then
    stat -c%s "$1"   # GNU coreutils (Linux)
  else
    stat -f%z "$1"   # BSD/macOS
  fi
}

mkdir -p compacted
echo "file,before,after,before_compressed,after_compressed,import_before,import_after"
for file in *.wasm; do
  go run .. $file -o compacted/$file
  gzip -k --force $file
  gzip -k --force compacted/$file

  before=$(get_size $file)
  after=$(get_size "compacted/$file")
  before_compressed=$(get_size $file.gz)
  after_compressed=$(get_size "compacted/$file.gz")
  import_before_hex=$({ wasm-objdump -h $file 2>&1 || true; } | awk '/Import/{print $4}' | sed 's/(size=0x//' | sed 's/)//')
  import_after_hex=$({ wasm-objdump -h "compacted/$file" 2>&1 || true; } | awk '/Import/{print $4}' | sed 's/(size=0x//' | sed 's/)//')
  import_before=$((16#$import_before_hex))
  import_after=$((16#$import_after_hex))

  echo "$file,$before,$after,$before_compressed,$after_compressed,$import_before,$import_after"
done
