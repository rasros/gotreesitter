# cgo_harness

This module contains CGo-only parity and baseline benchmark harnesses used to compare `gotreesitter` against native C tree-sitter parsers.

## Run Parity Tests

```sh
go test . -tags treesitter_c_parity -run TestParity -v
```

## Run C Baseline Benchmarks

```sh
go test . -run '^$' -tags treesitter_c_bench -bench BenchmarkCTreeSitter -benchmem
```

These harnesses are intentionally split into a separate module so the root `gotreesitter` module remains pure-Go in dependency metadata.
