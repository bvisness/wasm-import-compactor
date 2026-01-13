#!/bin/bash
set -euxo pipefail

for file in compacted/*.wasm; do
  js -e "new WebAssembly.Module(read('$file', 'binary'))"
done
