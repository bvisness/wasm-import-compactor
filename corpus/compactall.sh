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
mkdir -p compacted2
mkdir -p binaryenated
mkdir -p binaryenated_compact
echo "file,before,after_enc1,after_enc2,binaryenated,binaryenated_compact,before_compressed,after_enc1_compressed,after_enc2_compressed,binaryenated_compressed,binaryenated_compact_compressed,import_before,import_after_enc1,import_after_enc2,import_binaryenated,import_binaryenated_compact,min_possible_enc1,min_possible_enc2"
for file in *.wasm; do
  go run .. $file -o compacted/$file --encoding-2=false --counts compacted/$file.counts.csv --min-possible compacted/$file.min.csv
  go run .. $file -o compacted2/$file --min-possible compacted2/$file.min.csv
  gzip -k --force $file
  gzip -k --force compacted/$file
  gzip -k --force compacted2/$file

  $WASM_OPT -O1 -all --strip-debug --generate-stack-ir --optimize-stack-ir --disable-compact-imports $file -o binaryenated/$file
  $WASM_OPT -O1 -all --strip-debug --generate-stack-ir --optimize-stack-ir $file -o binaryenated_compact/$file
  gzip -k --force binaryenated/$file
  gzip -k --force binaryenated_compact/$file

  before=$(get_size $file)
  after_enc1=$(get_size "compacted/$file")
  after_enc2=$(get_size "compacted2/$file")
  before_compressed=$(get_size $file.gz)
  after_enc1_compressed=$(get_size "compacted/$file.gz")
  after_enc2_compressed=$(get_size "compacted2/$file.gz")
  binaryenated=$(get_size "binaryenated/$file")
  binaryenated_compact=$(get_size "binaryenated_compact/$file")
  binaryenated_compressed=$(get_size "binaryenated/$file.gz")
  binaryenated_compact_compressed=$(get_size "binaryenated_compact/$file.gz")
  import_before_hex=$({ wasm-objdump -h $file 2>&1 || true; } | awk '/Import/{print $4}' | sed 's/(size=0x//' | sed 's/)//')
  import_after_enc1_hex=$({ wasm-objdump -h "compacted/$file" 2>&1 || true; } | awk '/Import/{print $4}' | sed 's/(size=0x//' | sed 's/)//')
  import_after_enc2_hex=$({ wasm-objdump -h "compacted2/$file" 2>&1 || true; } | awk '/Import/{print $4}' | sed 's/(size=0x//' | sed 's/)//')
  import_binaryenated_hex=$({ wasm-objdump -h "binaryenated/$file" 2>&1 || true; } | awk '/Import/{print $4}' | sed 's/(size=0x//' | sed 's/)//')
  import_binaryenated_compact_hex=$({ wasm-objdump -h "binaryenated_compact/$file" 2>&1 || true; } | awk '/Import/{print $4}' | sed 's/(size=0x//' | sed 's/)//')
  import_before=$((16#$import_before_hex))
  import_after_enc1=$((16#$import_after_enc1_hex))
  import_after_enc2=$((16#$import_after_enc2_hex))
  import_binaryenated=$((16#$import_binaryenated_hex))
  import_binaryenated_compact=$((16#$import_binaryenated_compact_hex))
  import_min_possible_enc1=$(<"compacted/$file.min.csv")
  import_min_possible_enc2=$(<"compacted2/$file.min.csv")

  echo "$file,$before,$after_enc1,$after_enc2,$binaryenated,$binaryenated_compact,$before_compressed,$after_enc1_compressed,$after_enc2_compressed,$binaryenated_compressed,$binaryenated_compact_compressed,$import_before,$import_after_enc1,$import_after_enc2,$import_binaryenated,$import_binaryenated_compact,$import_min_possible_enc1,$import_min_possible_enc2"
done
