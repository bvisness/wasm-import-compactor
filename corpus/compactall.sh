#!/usr/bin/env bash
set -euo pipefail

WASM_OPT=${WASM_OPT:-/Users/mozilla/Developer/WebAssembly/binaryen/bin/wasm-opt}

get_size() {
  if stat --version >/dev/null 2>&1; then
    stat -c%s "$1"   # GNU coreutils (Linux)
  else
    stat -f%z "$1"   # BSD/macOS
  fi
}

mkdir -p compacted
mkdir -p binaryenated
mkdir -p binaryenated_compact
echo "file,before,after,binaryenated,binaryenated_compact,before_compressed,after_compressed,binaryenated_compressed,binaryenated_compact_compressed,import_before,import_after,import_binaryenated,import_binaryenated_compact"
for file in *.wasm; do
  go run .. $file -o compacted/$file --counts compacted/$file.counts.csv
  gzip -k --force $file
  gzip -k --force compacted/$file

  $WASM_OPT -Oz --strip-debug -all --disable-compact-imports $file -o binaryenated/$file
  $WASM_OPT -Oz --strip-debug -all $file -o binaryenated_compact/$file
  gzip -k --force binaryenated/$file
  gzip -k --force binaryenated_compact/$file

  before=$(get_size $file)
  after=$(get_size "compacted/$file")
  before_compressed=$(get_size $file.gz)
  after_compressed=$(get_size "compacted/$file.gz")
  binaryenated=$(get_size "binaryenated/$file")
  binaryenated_compact=$(get_size "binaryenated_compact/$file")
  binaryenated_compressed=$(get_size "binaryenated/$file.gz")
  binaryenated_compact_compressed=$(get_size "binaryenated_compact/$file.gz")
  import_before_hex=$({ wasm-objdump -h $file 2>&1 || true; } | awk '/Import/{print $4}' | sed 's/(size=0x//' | sed 's/)//')
  import_after_hex=$({ wasm-objdump -h "compacted/$file" 2>&1 || true; } | awk '/Import/{print $4}' | sed 's/(size=0x//' | sed 's/)//')
  import_binaryenated_hex=$({ wasm-objdump -h "binaryenated/$file" 2>&1 || true; } | awk '/Import/{print $4}' | sed 's/(size=0x//' | sed 's/)//')
  import_binaryenated_compact_hex=$({ wasm-objdump -h "binaryenated_compact/$file" 2>&1 || true; } | awk '/Import/{print $4}' | sed 's/(size=0x//' | sed 's/)//')
  import_before=$((16#$import_before_hex))
  import_after=$((16#$import_after_hex))
  import_binaryenated=$((16#$import_binaryenated_hex))
  import_binaryenated_compact=$((16#$import_binaryenated_compact_hex))

  echo "$file,$before,$after,$binaryenated,$binaryenated_compact,$before_compressed,$after_compressed,$binaryenated_compressed,$binaryenated_compact_compressed,$import_before,$import_after,$import_binaryenated,$import_binaryenated_compact"
done
