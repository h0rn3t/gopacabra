# Third-party notices

LLGo is licensed under Apache License 2.0. The following components retain
their own licenses. The referenced license files are included in source
checkouts, release archives, and packaged distributions. The nested `runtime`
module carries its own copy of LLGo's Apache-2.0 license in `runtime/LICENSE`.

## TinyGo

LLGo contains source derived from the [TinyGo project](https://github.com/tinygo-org/tinygo):

- the deprecated archive builder in `chore/_deprecated/ar/ar.go`;
- target configurations and support files under `targets`.

Reference snapshots contemporaneous with the initial LLGo imports are:

- [runtime GC](https://github.com/tinygo-org/tinygo/tree/79ab77facd8b4d7ea39257f85d37f094f52770d2/src/runtime);
- [initial target configuration import](https://github.com/tinygo-org/tinygo/tree/8c5886060f022a36768b5c29327759846021a868/targets).

The TinyGo-derived portions remain subject to the BSD 3-Clause License in
[`LICENSES/TinyGo-BSD-3-Clause.txt`](LICENSES/TinyGo-BSD-3-Clause.txt).
Independently written LLGo code remains subject to the repository's Apache
License 2.0.

## The Go project

LLGo distributes Go-derived source under `runtime`, `targets/wasm_exec.js`,
`chore/ssadump`, and selected compatibility tests.
The `llgo` executable also contains the Go standard library and packages from
`golang.org/x/mod`, `golang.org/x/sync`, `golang.org/x/sys`, and
`golang.org/x/tools`. The authoritative versions are recorded in `go.mod` and
the executable's Go build information.

These portions are licensed under the Go project's BSD 3-Clause License in
[`LICENSES/Go-BSD-3-Clause.txt`](LICENSES/Go-BSD-3-Clause.txt). The nested
runtime module also carries a copy at
[`runtime/LICENSES/Go-BSD-3-Clause.txt`](runtime/LICENSES/Go-BSD-3-Clause.txt).

## Source incorporated into LLGo

| Component | Distributed in | License |
| --- | --- | --- |
| `github.com/blakesmith/ar` and LLGo adaptations | `xtool/ar`, source and development tools | [MIT](LICENSES/BlakeSmith-AR-MIT.txt) |

## Go modules compiled into the llgo executable

This list is limited to modules in the shipped Darwin and Linux
executables; development- and test-only modules are not included.

| Module | License |
| --- | --- |
| `github.com/goplus/cobra` | [Apache-2.0](LICENSES/Cobra-Apache-2.0.txt); compiled `pflag` package: [BSD-3-Clause](LICENSES/Cobra-pflag-BSD-3-Clause.txt) |
| `github.com/qiniu/x` | [Apache-2.0](LICENSES/Qiniu-X-Apache-2.0.txt) |
| `github.com/xgo-dev/llvm` | [Apache-2.0 WITH LLVM-exception](LICENSES/XGo-LLVM-Apache-2.0-WITH-LLVM-exception.txt) |
| `github.com/xgo-dev/plan9asm` | [Apache-2.0](LICENSES/XGo-Plan9Asm-Apache-2.0.txt) |
| `go.yaml.in/yaml/v3` | [MIT and Apache-2.0](LICENSES/Go-YAML-LICENSE.txt), [NOTICE](LICENSES/Go-YAML-NOTICE.txt) |
| `golang.org/x/mod`, `x/sync`, `x/sys`, and `x/tools` | [BSD-3-Clause](LICENSES/Go-BSD-3-Clause.txt) |

## LLVM/Clang

LLGo can download the Espressif-maintained ESP LLVM/Clang 22 toolchain
[`22.1.4_20260905`](https://github.com/goplus/espressif-llvm-project-prebuilt/releases/tag/22.1.4_20260905).

Current LLGo release archives use the LLVM 22 payload and include it under
`crosscompile/clang`. Darwin and Linux executables dynamically link its LLVM
library. Each ESP payload includes `THIRD-PARTY-LICENSES.txt`
and the component license texts for LLVM, Clang, LLD, compiler-rt, libc++,
libc++abi, and libunwind.

LLVM, Clang, LLD, libc++, libc++abi, libunwind, compiler-rt, and other
LLVM-project components are licensed under Apache License 2.0 with LLVM
Exceptions. The complete license is reproduced in
[`LICENSES/XGo-LLVM-Apache-2.0-WITH-LLVM-exception.txt`](LICENSES/XGo-LLVM-Apache-2.0-WITH-LLVM-exception.txt).
Archive extraction preserves upstream license files.

## Components downloaded for cross-compilation

LLGo downloads the following components directly from their upstream release
locations when a selected target needs them. They are stored in the user's LLGo
cache; they are not vendored in this source tree. Their license files remain in
the extracted source or SDK directory.

| Component | Download source | License location |
| --- | --- | --- |
| WASI SDK 25 | [`WebAssembly/wasi-sdk`](https://github.com/WebAssembly/wasi-sdk/releases/tag/wasi-sdk-25) | upstream `LICENSE` and license files in the SDK |

## External tools and system libraries

OpenSSL, libffi, cJSON, SQLite, zlib, Python, Emscripten, WebAssembly
runtimes, and platform SDK/system libraries are installed or
provided separately. LLGo may link to or invoke them, but does not copy them
into its source tree or release archives, except for LLVM-project components
explicitly described above. Their upstream licenses therefore apply to the
separate installations and to any redistributed output that incorporates them;
they are not relicensed by LLGo.

## Redistribution

Distributions containing the source, the `llgo` executable, the bundled
toolchain, or programs linked with third-party runtime or target support must
retain or reproduce the applicable copyright notices, license conditions,
disclaimers, and NOTICE text described above.
