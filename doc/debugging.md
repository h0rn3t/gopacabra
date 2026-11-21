# LLGo debug sessions

Native Linux links compress retained DWARF with zlib by default. Use
`-ldflags=-compressdwarf=false` to keep it uncompressed; `-ldflags=-w` still
omits DWARF entirely. Compression retains the ordinary symbol table and the
runtime PCLN metadata.

Native macOS also accepts **experimental, explicit**
`-ldflags=-compressdwarf=true` compression. Their default remains uncompressed.
On macOS this runs `dsymutil` and compresses the adjacent `program.dSYM`
bundle's DWARF using GNU `__zdebug_*` sections; the executable and its code
signature are unchanged. Unsupported layouts are rejected before rewriting
the image. `-w` still omits DWARF and
does not generate a dSYM.

These formats can be read by Go's `debug/macho` package, but
reader support varies. In particular the tested Apple LLDB cannot read
compressed Mach-O dSYMs. `llgo debug` with its LLDB backend rejects explicit compression
on either platform. For LLDB use the default or `-compressdwarf=false`. If a
compressed bundle already exists, regenerate an uncompressed one with
`dsymutil program -o program.dSYM` while its debug-map objects remain
available. If the executable was stripped, rebuild with
`-ldflags='-s -w=false -compressdwarf=false'` instead, because its debug map
has already been discarded. Treat the compressed dSYM as a storage/reader experiment until
the debugger supports it. Successful DWARF decoding does not establish
compatibility with other debuggers.

Rebuilding without explicit compression (the default), with compression
disabled, or with DWARF omitted removes an earlier LLGo-generated compressed
bundle so it cannot mask the new image's debug map. This rebuild
cleanup leaves manually generated bundles alone. `llgo clean` removes the
default output's adjacent dSYM bundle, and temporary
`run`/`test` outputs reclaim generated bundles automatically. When using a
custom `-o program`, dispose of `program.dSYM` along with the executable.
With `-ldflags='-s -w=false -compressdwarf=true'`, compression runs on the
staged dSYM before symbol stripping publishes the executable and bundle.
This keeps the matching DWARF even though stripping discards the debug map.

On macOS, size-optimized main executables limit automatic dynamic exports
even when DWARF is enabled. Explicit exports, shared libraries, and dynamic
lookup configurations keep their export policy. DWARF builds retain their
function exports when the embedded runtime table needs `dlsym`; data and type
definitions are hidden in non-LTO builds. LTO with DWARF keeps its existing
visibility policy. Builds with function-entry sites can suppress all automatic
exports. DWARF builds retain their debug maps, so `dsymutil program`
can generate a separate `program.dSYM`.
Full/ThinLTO debug objects are retained in an adjacent
`program.llgo-debug-lto/` directory for this purpose. Each link retains a
separate object so rebuilding cannot invalidate a copied older executable.
Repeated explicit builds therefore retain historical objects until cleanup:
`llgo clean` removes the default output's directory, and temporary `run`/`test`
outputs reclaim it automatically. For a custom `-o program`, remove
`program.llgo-debug-lto/` when disposing of its debug maps. Generate any dSYMs
needed by retained executable copies before cleaning these objects.

For native Linux and macOS executables and shared libraries, `-ldflags=-s`
removes ordinary symbols after runtime PCLN packaging and implies `-w`. Use
`-ldflags='-s -w=false'` to retain source-level debugging, including the few
symbols used for debugger ABI/registry discovery. Linux retains its DWARF
sections; macOS generates a matching adjacent `.dSYM` before removing
the executable's debug map and re-signing. Build artifact reports include
the complete dSYM bundle. Shared-library dynamic exports are preserved.
`llgo clean` also removes the default output's adjacent dSYM bundle.
Archives, Wasm, fixed targets, and other native object formats retain their
existing symbol policy.

`llgo debug` is the target-aware entry point for building and debugging an
LLGo program:

```sh
llgo debug [build flags] [package] [-- debugger arguments...]
```

It enables DWARF, uses `-O0` unless an optimization level was selected, builds
one executable package, and owns any temporary artifact and local debug-server
process. An explicit `-o` keeps the artifact. `-ldflags=-w` and
`-debug-artifact=none` are rejected because the resulting program cannot be
source-debugged.

The automatic backend depends on the selected target:

| Target | Backend | Session transport |
| --- | --- | --- |
| Native Darwin/Linux | LLDB | Local process |
| WASI | Unavailable | W32 pthread execution uses Wasmer; source debugging is a subsequent phase |
| Browser Wasm | Chrome DevTools | LLGo language extension and the generated Emscripten host |

Use `-backend=gdb` or `-backend=lldb` to override a native or GDB Remote
session, and `-gdb` or `-lldb` to select a debugger executable. For an already
running server, `-remote=host:port` skips server startup. A target's
`debug-server` command can use `{}` or `{elf}` for the host debug artifact and
`{debug-stdio}` to select the server's stdin/stdout RSP transport. LLGo retains
an OS-assigned loopback listener and relays the debugger connection to that
transport without a TCP readiness probe or a released port reservation.

Legacy TCP templates can use `{debug-port}`. Their server-side allocation is
best effort; use the stdio transport to avoid the reserve/rebind race. Their
first connection is retained as the RSP transport, rather than discarded as
an empty probe.

An existing `-remote` or custom `-server` is not written to by default. Add
`-load` to request a reset/download/reset sequence; this requires a server
that implements the `monitor reset halt` command and image writes.

Browser sessions use the separate DevTools frontend contribution; the current
threaded WASI runtime does not gain a source-debugger frontend from remote
debugging.

Native runtime inspection has additional debugger-specific limits. Current
GDB acceptance on the following hosts is partial; use LLDB when complete
goroutine stack inspection is required:

| Native host | GDB results with the current packaged tool | LLDB results |
| --- | --- | --- |
| macOS x64 with dyld image-info ABI 17 | C/Go values, runtime registry and main-thread stack pass; blocked-worker stacks are truncated because GDB's Darwin loader reader accepts image-info ABI versions only through 15. The truncation also reproduces with a pure C worker. | Complete runtime acceptance passes, including worker stacks |

Passing value inspection or enumerating a goroutine does not establish that
its full stack can be unwound. This GDB combination is not recorded as
complete native runtime support, and this contribution does not bundle a
patched GDB to remove those upstream limits.

`llgo lldb` remains the explicit compatibility command for opening an existing
artifact without building it.

For browser source debugging, use Chrome for Testing or Chromium with the
unpacked-extension command-line flags enabled:

```sh
llgo debug -target=emscripten -chrome /path/to/chrome ./app
llgo debug -target=emscripten-memory64 -debug-artifact=external ./app
GOOS=js GOARCH=wasm llgo debug ./app
```

The browser session starts a loopback HTTP server, loads an isolated Chrome
profile with the LLGo extension, and opens DevTools. Set breakpoints in Sources
and use the session page to run the program. `-source-map=FROM=TO` is repeatable
and maps compiler-recorded paths to local source files; `-browser-devtools=false`
disables the extension handshake and allows execution without DevTools. Browser
arguments after `--` are limited to `--window-size`, `--window-position`,
`--start-maximized` and `--headless`; provide values as `--name=value`.
Profile, extension and web security settings remain owned by the session.
If the extension does not become ready within five seconds, the page reports
an error before instantiation; open DevTools and reload to retry. Sources are
served only from this build's package directories, the artifact directory, or
explicit source-map destinations. Unmapped paths outside these roots remain
unavailable. Each session accepts only its own extension's CORS origin.
`-remote` and `-server` belong to native GDB Remote transports and are rejected for
browser sessions. The generated Emscripten host, profile and worker settings
are preserved; the launcher does not substitute host imports.

Browser debugging supports paused Wasm locals and an opt-in `$goroutines`
watch view of logical Go source stacks, including parked fibers. The session
coordinates pauses, resumes and stepping across the page and its dedicated
workers. Shared-memory inspection waits for a stable group stop and rejects
values if the stop or worker membership changes. Expand a waiting goroutine's
frame to inspect memory locals whose DWARF scope and frame-relative location
remain valid throughout the function. Register values, changing locations and
panic snapshots remain unavailable.
WASI source sessions are explicitly rejected: current W32 modules require
shared `env.memory` and `wasi.thread-spawn`, which the
current Wasmtime debug backend does not implement. Use `llgo run -target=wasi`
with Wasmer for execution; retaining valid DWARF does not provide a runtime
debugger transport. See [browser debugging](../dev/browser-debugging.md).

Debug information can also be packaged without starting a debugger:

```sh
llgo build -ldflags=-w=false ./app
llgo build -target=emscripten -debug-artifact=embedded -o app.mjs ./app
llgo build -target=emscripten -debug-artifact=external -o app.mjs ./app
llgo build -debug-artifact=none ./app
```

Native macOS and Linux builds follow the platform toolchain's default
DWARF packaging. Retaining DWARF does not prescribe whether it lives in the
executable, object files or a separate debug artifact. LLGo does not add a native
packaging step or require a standalone dSYM/debug file. On native targets,
`-debug-artifact=embedded` only requests DWARF preservation; it does not promise
a self-contained executable. Use `-ldflags=-w=false` when only preservation is
needed. Explicit embedded/external packaging is defined for Wasm.

`external` currently applies to Wasm executables. It writes a sibling
`app.debug.wasm`, leaves executable code/data sections and JavaScript glue
unchanged, and records the same build ID and debugger ABI in both modules.
Artifact reports list their separate byte sizes; runtime `.pclntab` sidecars
retain an independent role.

Wasm artifact acceptance uses `dev/test_wasm_debug_info.py --artifact embedded
--artifact external`; it validates packaging and execution rather than debugger session capabilities.
