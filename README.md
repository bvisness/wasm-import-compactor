# wasm-import-compactor

Updates wasm modules to use the proposed [compact import encoding](https://github.com/WebAssembly/compact-import-section). It does not reorder any imports, so all indices remain unchanged. As a result, the compacting is "RLE-style"—only adjacent imports will be compacted.

## Running

Go 1.23.4 or greater is required.

```
$ go run main.go
Usage:
  wasm-import-compactor <file> [flags]

Flags:
  -h, --help         help for wasm-import-compactor
  -o, --out string   The file to write output to. Defaults to stdout. (default "-")

$ go run main.go original.wasm -o compacted.wasm
```
