# Debugger acceptance tests

This directory contains integration suites that invoke real debugger tools.
They run explicitly in CI and require an installed LLGo compiler and the tools
listed by each suite.

For debugger usage and artifact options, see the
[debugging guide](../../doc/debugging.md). This directory documents acceptance
entry points and their tool requirements.

- [Runtime inspection](runtime/README.md) shares native Go and C fixtures between
  LLDB and GDB, covering variables, runtime values, goroutines, and backtraces.
- [Wasm logical stacks](wasm/README.md) exercises the installed Chrome extension
  with Memory32/64, parked goroutines, GC, and 1/2/4 workers.

Run the native LLDB suite from the repository root:

```sh
bash test/debug/runtime/runtest.sh -v
```

Run GDB's native values, registry and main-stack acceptance with Python-enabled
GDB 12+ (the Windows ARM64 jobs use GDB 18+):

```sh
LLGO_GDB_INTEGRATION=1 go test ./cmd/internal/gdb -run '^TestGDBIntegration$' -count=1 -v
```

Complete blocked-worker unwind is a separate strict acceptance test:

```sh
LLGO_GDB_INTEGRATION=1 go test ./cmd/internal/gdb -run '^TestGDBCompleteWorkerUnwind$' -count=1 -v
```

The latter remains a required gate on Linux and Windows amd64/386. Stock GDB
cannot currently complete that test on Windows ARM64 (PAC masks), although
native values, the goroutine registry and the main application stack are
tested there. See the
[runtime coverage matrix](runtime/README.md#gdb-runtime-acceptance). These
limitations do not apply to the native LLDB suite.

The runtime fixtures retain their own `go.mod` files, including
the `lldbtest` module name used by runtime debugger type assertions. The nested
modules exclude these programs
from root-module `go test ./test/...` and `llgo test ./test/...` enumeration.
The LLGo workflow invokes the LLDB suite in its native platform jobs and the
GDB suite in its Linux and all six Windows ABI/architecture jobs.
The Windows debugger is installed separately from the pinned compiler and
LLDB dependencies. Apple Silicon has no native GDB process target: use LLDB
there; GDB remote debugging remains available. Unit tests and package-specific `testdata` remain
beside their implementation.
