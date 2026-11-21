package crosscompile

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/xgo-dev/llgo/internal/env"
	"github.com/xgo-dev/llgo/internal/lto"
	"github.com/xgo-dev/llgo/internal/optlevel"
	"github.com/xgo-dev/llgo/internal/xtool/llvm"
	gollvm "github.com/xgo-dev/llvm"
)

const (
	sysrootPrefix     = "--sysroot="
	resourceDirPrefix = "-resource-dir="
	includePrefix     = "-I"
	libPrefix         = "-L"
)

// ESP clang компілює IR, який генерує компілятор, тому його мажорна версія
// має збігатися з версією LLVM, з якою злінковано компілятор: старший clang
// не розбирає новий синтаксис IR (наприклад, `double +inf` з LLVM 23).
func TestESPClangMatchesLLVMMajor(t *testing.T) {
	espMajor, _, _ := strings.Cut(espClangVersion, ".")
	llvmMajor, _, _ := strings.Cut(gollvm.Version, ".")
	if espMajor != llvmMajor {
		t.Errorf("ESP Clang %s, LLVM %s: major versions differ", espClangVersion, gollvm.Version)
	}
}

func TestESPClangHostDownload(t *testing.T) {
	tests := []struct {
		goos, goarch string
		wantPlatform string
	}{
		{"darwin", "arm64", "aarch64-apple-darwin"},
		{"linux", "arm64", "aarch64-linux-gnu"},
		{"linux", "amd64", "x86_64-linux-gnu"},
	}
	for _, test := range tests {
		platform := getESPClangPlatform(test.goos, test.goarch)
		if platform != test.wantPlatform {
			t.Errorf("getESPClangPlatform(%q, %q) = %q, want %q", test.goos, test.goarch, platform, test.wantPlatform)
			continue
		}
		if espClangSHA256[platform] == "" {
			t.Errorf("ESP Clang %s %s has no checksum", espClangVersion, platform)
		}
	}
}

func TestESPClangCacheSeparatesHosts(t *testing.T) {
	legacy := filepath.Join(cacheDir(), "esp-clang-"+espClangVersion)
	x64 := espClangCacheDir(getESPClangPlatform("linux", "amd64"))
	arm64 := espClangCacheDir(getESPClangPlatform("linux", "arm64"))
	if x64 == arm64 || x64 == legacy || arm64 == legacy {
		t.Fatalf("ESP payload caches overlap: x64=%q arm64=%q legacy=%q", x64, arm64, legacy)
	}
}

func TestESPClangDoesNotReuseLegacyCache(t *testing.T) {
	writeWasmTargetFixture(t, "", "")
	originalCacheRoot, originalBaseURL := cacheRoot, espClangBaseUrl
	root := t.TempDir()
	cacheRoot = func() string { return root }
	server := httptest.NewServer(http.NotFoundHandler())
	espClangBaseUrl = server.URL
	t.Cleanup(func() {
		cacheRoot, espClangBaseUrl = originalCacheRoot, originalBaseURL
		server.Close()
	})
	legacy := filepath.Join(cacheDir(), "esp-clang-"+espClangVersion)
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := getESPClangRoot(true)
	if got != "" || err == nil || !strings.Contains(err.Error(), "404 Not Found") {
		t.Fatalf("getESPClangRoot() = %q, %v; want a fresh download instead of legacy cache %q", got, err, legacy)
	}
}

func TestESPClangRejectsMissingChecksum(t *testing.T) {
	writeWasmTargetFixture(t, "", "")
	checksums := espClangSHA256
	espClangSHA256 = nil
	t.Cleanup(func() { espClangSHA256 = checksums })

	root, err := getESPClangRoot(true)
	platform := getESPClangPlatform(runtime.GOOS, runtime.GOARCH)
	if platform == "" {
		if err == nil || !strings.Contains(err.Error(), "is not supported for download") {
			t.Fatalf("unsupported host error = %v", err)
		}
		return
	}
	if root != "" || err == nil || err.Error() != "missing ESP Clang checksum for "+platform {
		t.Fatalf("getESPClangRoot() = %q, %v; want missing checksum error", root, err)
	}
}

func TestUseCrossCompileSDK(t *testing.T) {
	// This test checks SDK flags, so use a local SDK fixture instead of
	// depending on a release download and external network availability.
	writeWasmTargetFixture(t, "", "")
	if err := os.MkdirAll(filepath.Join(os.Getenv("LLGO_ROOT"), "crosscompile", "wasi-libc"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Test cases
	testCases := []struct {
		name          string
		goos          string
		goarch        string
		expectSDK     bool
		expectCCFlags bool
		expectCFlags  bool
		expectLDFlags bool
	}{
		{
			name:          "Same Platform",
			goos:          runtime.GOOS,
			goarch:        runtime.GOARCH,
			expectSDK:     true,  // We expect flags even for same platform
			expectCCFlags: true,  // CCFLAGS will contain sysroot
			expectCFlags:  false, // CFLAGS will not contain include paths
			expectLDFlags: false, // LDFLAGS will not contain library paths
		},
		{
			name:          "WASM Target",
			goos:          "wasip1",
			goarch:        "wasm",
			expectSDK:     true,
			expectCCFlags: true,
			expectCFlags:  true,
			expectLDFlags: true,
		},
		{
			name:          "Unsupported Target",
			goos:          "windows",
			goarch:        "amd64",
			expectSDK:     false, // Still false as it won't set up specific SDK
			expectCCFlags: false, // No cross-compile specific flags
			expectCFlags:  false, // No cross-compile specific flags
			expectLDFlags: false, // No cross-compile specific flags
		},
	}

	// Create a temporary directory for the cache
	tempDir, err := os.MkdirTemp("", "crosscompile_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalCacheRoot := cacheRoot
	cacheRoot = func() string { return tempDir }
	defer func() { cacheRoot = originalCacheRoot }()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			export, err := use(tc.goos, tc.goarch, false, optlevel.O2, lto.Off, false)

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			t.Logf("export: %+v", export)

			if tc.expectSDK {
				// Check if flags are set correctly
				if tc.expectCCFlags && len(export.CCFLAGS) == 0 {
					t.Error("Expected CCFLAGS to be set, but they are empty")
				}

				if tc.expectCFlags && len(export.CFLAGS) == 0 {
					t.Error("Expected CFLAGS to be set, but they are empty")
				}

				if tc.expectLDFlags && len(export.LDFLAGS) == 0 {
					t.Error("Expected LDFLAGS to be set, but they are empty")
				}

				// Check for specific flags
				if tc.expectCCFlags {
					hasSysroot := false
					hasResourceDir := false

					for _, flag := range export.CCFLAGS {
						if len(flag) >= len(sysrootPrefix) && flag[:len(sysrootPrefix)] == sysrootPrefix {
							hasSysroot = true
						}
						if len(flag) >= len(resourceDirPrefix) && flag[:len(resourceDirPrefix)] == resourceDirPrefix {
							hasResourceDir = true
						}
					}

					// For WASM target, both sysroot and resource-dir are expected
					if tc.name == "WASM Target" {
						if !hasSysroot {
							t.Error("Missing --sysroot flag in CCFLAGS")
						}
						if !hasResourceDir {
							t.Error("Missing -resource-dir flag in CCFLAGS")
						}
						if !slices.Contains(export.CCFLAGS, "-fwasm-exceptions") ||
							!hasMllvmOption(export.CCFLAGS, "-wasm-enable-sjlj") ||
							!hasMllvmOption(export.CCFLAGS, "-wasm-use-legacy-eh=false") {
							t.Errorf("CCFLAGS do not enable WebAssembly SjLj lowering: %v", export.CCFLAGS)
						}
						if export.WasmPostLink.Asyncify {
							t.Error("WASI threads unexpectedly request Asyncify")
						}
						if !slices.Contains(export.LDFLAGS, "-Wl,--import-memory") {
							t.Errorf("WASI threads must import shared memory: %v", export.LDFLAGS)
						}
						if !slices.Contains(export.LDFLAGS, "-Wl,--stack-first") {
							t.Errorf("WASI does not fix the process stack layout: %v", export.LDFLAGS)
						}
					} else if tc.name == "Same Platform" {
						// For same platform, we expect sysroot only on macOS
						if runtime.GOOS == "darwin" && !slices.Contains(export.CCFLAGS, "-isysroot") {
							t.Error("Missing -isysroot flag in CCFLAGS on macOS")
						}
						// On Linux and other platforms, sysroot is not necessarily required
					}
				}

				if tc.expectCFlags {
					hasInclude := false

					for _, flag := range export.CFLAGS {
						if len(flag) >= len(includePrefix) && flag[:len(includePrefix)] == includePrefix {
							hasInclude = true
						}
					}

					if !hasInclude {
						t.Error("Missing -I flag in CFLAGS")
					}
				}

				if tc.expectLDFlags {
					hasLib := false

					for _, flag := range export.LDFLAGS {
						if len(flag) >= len(libPrefix) && flag[:len(libPrefix)] == libPrefix {
							hasLib = true
						}
					}

					if !hasLib {
						t.Error("Missing -L flag in LDFLAGS")
					}
				}
			} else {
				// For unsupported targets, we still expect some basic flags to be set
				// since the implementation now always sets up ESP Clang environment
				// Only check that we don't have specific SDK-related flags for unsupported targets
				if tc.name == "Unsupported Target" && len(export.CFLAGS) != 0 {
					t.Errorf("Expected empty CFLAGS for unsupported target, got CFLAGS=%v", export.CFLAGS)
				}
			}
		})
	}
}

func TestUseWASIThreadsImportsMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the external WASI SDK link test in short mode")
	}
	export, err := use("wasip1", "wasm", false, optlevel.O2, lto.Off, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(export.CCFLAGS, "-pthread") {
		t.Fatalf("CCFLAGS do not enable WASI threads: %v", export.CCFLAGS)
	}
	if i := slices.Index(export.CCFLAGS, "-target"); i < 0 || i+1 == len(export.CCFLAGS) || export.CCFLAGS[i+1] != "wasm32-wasip1-threads" {
		t.Fatalf("CCFLAGS do not select the WASI threads startup files: %v", export.CCFLAGS)
	}
	if i := slices.Index(export.LDFLAGS, "-target"); i < 0 || i+1 == len(export.LDFLAGS) || export.LDFLAGS[i+1] != "wasm32-wasip1-threads" {
		t.Fatalf("LDFLAGS do not select the WASI threads startup files: %v", export.LDFLAGS)
	}
	if !slices.Contains(export.BuildTags, "llgo.wasi_threads") {
		t.Fatalf("BuildTags do not select the WASI pthread backend: %v", export.BuildTags)
	}
	if !slices.Contains(export.LDFLAGS, "-Wl,--import-memory") {
		t.Fatalf("LDFLAGS do not import shared host memory: %v", export.LDFLAGS)
	}
	if slices.Contains(export.LDFLAGS, "-lwasi-emulated-pthread") {
		t.Fatalf("LDFLAGS select the single-thread pthread shim: %v", export.LDFLAGS)
	}
	if slices.Contains(export.LDFLAGS, "-Wl,--initial-memory=67108864") {
		t.Fatalf("WASI pthread memory must include static data in addition to the default heap: %v", export.LDFLAGS)
	}
	if !slices.Contains(export.LDFLAGS, "-Wl,--max-memory=536870912") {
		t.Fatalf("WASI pthread memory has no room for additional GC arenas: %v", export.LDFLAGS)
	}
	if export.WasmPostLink.Asyncify {
		t.Fatal("WASI pthread mode requests single-worker Asyncify processing")
	}
	named, err := Use("", "", "wasi", false, optlevel.O2, lto.Off, false)
	if err != nil {
		t.Fatal(err)
	}
	if named.Emulator != WASIThreadedEmulator {
		t.Fatalf("named WASI thread emulator = %q, want %q", named.Emulator, WASIThreadedEmulator)
	}
}

func TestUseWASILTOEnablesSjLjAtLink(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the external WASI SDK link test in short mode")
	}
	for _, mode := range []lto.Mode{lto.Off, lto.Thin, lto.Full} {
		t.Run(mode.String(), func(t *testing.T) {
			for _, tc := range []struct {
				level optlevel.Level
				want  string
			}{
				{optlevel.O0, "--lto-O0"}, {optlevel.O1, "--lto-O1"},
				{optlevel.O2, "--lto-O2"}, {optlevel.O3, "--lto-O3"},
				{optlevel.Os, "--lto-O2"}, {optlevel.Oz, "--lto-O2"},
			} {
				t.Run(tc.level.String(), func(t *testing.T) {
					export, err := use("wasip1", "wasm", false, tc.level, mode, false)
					if err != nil {
						t.Fatal(err)
					}
					for _, flags := range [][]string{export.CCFLAGS, export.LDFLAGS} {
						for _, candidate := range []lto.Mode{lto.Thin, lto.Full} {
							if slices.Contains(flags, candidate.ClangFlag()) != (mode == candidate) {
								t.Fatalf("LTO mode %s: incorrect %s in %v", mode, candidate.ClangFlag(), flags)
							}
						}
					}
					if hasFlagValue(export.LDFLAGS, "-Xlinker", "--mllvm=-mattr=+atomics,+bulk-memory,+exception-handling") != mode.Enabled() {
						t.Fatalf("LTO mode %s: missing shared-memory features in %v", mode, export.LDFLAGS)
					}
					for _, flag := range []string{"-Wl,--mllvm=-wasm-enable-sjlj", "-Wl,--mllvm=-exception-model=wasm", "-Wl,--mllvm=-wasm-use-legacy-eh=false"} {
						if slices.Contains(export.LDFLAGS, flag) != mode.Enabled() {
							t.Fatalf("LTO mode %s: incorrect %s in %v", mode, flag, export.LDFLAGS)
						}
					}
					var optimizationFlags []string
					for _, flag := range export.LDFLAGS {
						if strings.HasPrefix(flag, "-Wl,--lto-O") {
							optimizationFlags = append(optimizationFlags, flag)
						}
					}
					if mode.Enabled() {
						if len(optimizationFlags) != 1 || optimizationFlags[0] != "-Wl,"+tc.want {
							t.Fatalf("LTO optimizer flags = %v, want %s", optimizationFlags, tc.want)
						}
					} else if len(optimizationFlags) != 0 {
						t.Fatalf("LTO disabled but optimizer flags present: %v", optimizationFlags)
					}
				})
			}
		})
	}
}

func TestEnableWasmReflectLibffi(t *testing.T) {
	export, err := Use("js", "wasm", "", false, optlevel.O2, lto.Off, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnableWasmReflectLibffi(&export); err != nil {
		t.Fatal(err)
	}
	flags := strings.Join(export.LDFLAGS, " ")
	for _, entry := range []string{"PushCallerLocationFrameWasm", "RecordCallerLocationWasm", "RecordPanicLocationWasm", "llgo_reflect_bind0_js", "llgo_reflect_bind1_js", "llgo_reflect_bindn_js"} {
		if !strings.Contains(flags, entry) {
			t.Errorf("missing Asyncify exclusion %q: %s", entry, flags)
		}
	}
	if !slices.Contains(export.LDFLAGS, emscriptenAsyncifyImports) || !slices.Contains(export.LDFLAGS, emscriptenAllowTableGrowth) {
		t.Fatal("libffi reflection changed C libffi imports or table growth")
	}
	if err := EnableWasmReflectLibffi(&Export{}); err == nil {
		t.Fatal("libffi reflection accepted a target without Asyncify")
	}
}

func TestEmscriptenTargetProfiles(t *testing.T) {
	tests := []struct {
		name        string
		wantProfile WasmProfile
		wantTriple  string
		wantTags    []string
		memory64    bool
	}{
		{"emscripten", WasmProfileJ32, "wasm32-unknown-emscripten", []string{"llgo.wasm.emscripten"}, false},
		{"emscripten-memory64", WasmProfileJ64, "wasm64-unknown-emscripten", []string{"llgo.wasm.emscripten", "llgo.wasm.emscripten.memory64"}, true},
		{"wasm", WasmProfileJ32, "wasm32-unknown-emscripten", []string{"llgo.wasm.emscripten"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			export, err := Use("", "", test.name, false, optlevel.O2, lto.Off, false)
			if err != nil {
				t.Fatal(err)
			}
			if export.WasmProfile != test.wantProfile || export.WasmProvider != WasmProviderEmscripten || export.LLVMTarget != test.wantTriple ||
				export.GOOS != "js" || export.GOARCH != "wasm" || export.CC != "emcc" {
				t.Fatalf("export = profile/provider %q/%q, LLVM %q, %s/%s, CC %q",
					export.WasmProfile, export.WasmProvider, export.LLVMTarget, export.GOOS, export.GOARCH, export.CC)
			}
			if !slices.Equal(export.DebugInfo.PreserveDriverFlags, []string{"-gdwarf-4"}) {
				t.Fatalf("Emscripten must retain linked DWARF: %+v", export.DebugInfo)
			}
			for _, tag := range test.wantTags {
				if !slices.Contains(export.BuildTags, tag) {
					t.Errorf("build tags %v do not contain %q", export.BuildTags, tag)
				}
			}
			ccMemory64 := slices.Contains(export.CCFLAGS, "wasm64-unknown-emscripten")
			ldMemory64 := slices.Contains(export.LDFLAGS, "wasm64-unknown-emscripten")
			if got := ccMemory64 && ldMemory64; got != test.memory64 {
				t.Errorf("wasm64 compile/link target present = %v, want %v; CCFLAGS=%v LDFLAGS=%v", got, test.memory64, export.CCFLAGS, export.LDFLAGS)
			}
			if !slices.Contains(export.CCFLAGS, "-O2") || !slices.Contains(export.LDFLAGS, "-O2") {
				t.Errorf("Emscripten optimization level is not consistent across compile/link: CCFLAGS=%v LDFLAGS=%v", export.CCFLAGS, export.LDFLAGS)
			}
			if !slices.Contains(export.LDFLAGS, "-sENVIRONMENT=web,worker,node") {
				t.Errorf("named target does not enable its Node emulator: %v", export.LDFLAGS)
			}
			if !slices.Contains(export.LDFLAGS, "-sSTACK_OVERFLOW_CHECK=2") {
				t.Fatalf("named target does not check Fiber stack bounds: %v", export.LDFLAGS)
			}
			if !slices.Contains(export.LDFLAGS, emscriptenAsyncifyImports) {
				t.Errorf("named target does not mark interruptible host wait and ffi_call_js as async: %v", export.LDFLAGS)
			}
			if !slices.Contains(export.LDFLAGS, emscriptenAsyncifyRemove) {
				t.Errorf("named target instruments non-suspending caller and allocator paths: %v", export.LDFLAGS)
			}
			if strings.Contains(strings.Join(export.LDFLAGS, " "), "llgo_reflect_bind") {
				t.Error("named target retains obsolete reflection Asyncify exclusions")
			}
			if !slices.Contains(export.LDFLAGS, "-sALLOW_TABLE_GROWTH=1") {
				t.Errorf("named target does not allow dynamic libffi closure entries: %v", export.LDFLAGS)
			}
			if !slices.Contains(export.LDFLAGS, "-sNODE_HOST_ENV=1") {
				t.Errorf("named target does not propagate Node environment variables: %v", export.LDFLAGS)
			}
			if !slices.Contains(export.LDFLAGS, "-sEXPORTED_RUNTIME_METHODS=cwrap,allocateUTF8,stringToUTF8,UTF8ToString,FS,setValue,getValue,ENV") {
				t.Errorf("named target does not expose its browser environment map: %v", export.LDFLAGS)
			}
			if !slices.Contains(export.LDFLAGS, "-sEXIT_RUNTIME=1") {
				t.Errorf("named target does not let fatal Asyncify programs exit: %v", export.LDFLAGS)
			}
			wantRunner := "emscripten-runner.mjs"
			if test.memory64 {
				wantRunner = "emscripten-memory64-runner.mjs"
			}
			if strings.Contains(export.Emulator, "{root}") || !strings.Contains(export.Emulator, wantRunner) ||
				!strings.Contains(export.Emulator, "{}") {
				t.Errorf("named target emulator was not resolved: %q", export.Emulator)
			}
		})
	}
}

func TestEmscriptenAsyncifyLinkOptimization(t *testing.T) {
	for _, level := range []optlevel.Level{optlevel.O3, optlevel.Os, optlevel.Oz} {
		t.Run(level.Name(), func(t *testing.T) {
			export, err := Use("", "", "emscripten", false, level, lto.Off, false)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(export.CCFLAGS, level.Flag()) {
				t.Fatalf("compiler flags %v do not preserve requested %s", export.CCFLAGS, level)
			}
			if !slices.Contains(export.LDFLAGS, level.Flag()) || slices.Contains(export.LDFLAGS, "-O2") {
				t.Fatalf("link flags %v do not preserve requested %s", export.LDFLAGS, level)
			}
			if slices.Contains(export.LDFLAGS, "-sASSERTIONS=1") {
				t.Fatalf("link flags use assertions to disable MetaDCE: %v", export.LDFLAGS)
			}
			if !slices.Contains(export.LDFLAGS, "-sEXPORT_ALL=1") {
				t.Fatalf("link flags changed the public Module export contract: %v", export.LDFLAGS)
			}
		})
	}
}

func TestWASIProfileTarget(t *testing.T) {
	for _, target := range []string{"wasi", "wasip1"} {
		t.Run(target, func(t *testing.T) {
			export, err := Use("", "", target, false, optlevel.O2, lto.Off, false)
			if err != nil {
				t.Fatal(err)
			}
			if export.WasmProfile != WasmProfileW32 || export.WasmProvider != WasmProviderWASI || export.LLVMTarget != "wasm32-unknown-wasip1" ||
				export.GOOS != "wasip1" || export.GOARCH != "wasm" {
				t.Fatalf("export = profile/provider %q/%q, LLVM %q, %s/%s", export.WasmProfile, export.WasmProvider, export.LLVMTarget, export.GOOS, export.GOARCH)
			}
			if !slices.Contains(export.BuildTags, "llgo.wasm.wasi") {
				t.Errorf("build tags %v do not contain llgo.wasm.wasi", export.BuildTags)
			}
			if !slices.Equal(export.DebugInfo.PreserveDriverFlags, []string{"-gdwarf-4"}) {
				t.Fatalf("WASI driver must retain linked DWARF: %+v", export.DebugInfo)
			}
			if !slices.Contains(export.LDFLAGS, "-Wl,--import-memory") {
				t.Fatalf("WASI threads must import shared memory: %v", export.LDFLAGS)
			}
			if export.WasmPostLink.Asyncify {
				t.Fatal("WASI threads unexpectedly request Asyncify")
			}
			if !slices.Contains(export.BuildTags, "llgo.wasi_threads") {
				t.Fatalf("WASI threads source tag missing: %v", export.BuildTags)
			}
			if export.Emulator != WASIThreadedEmulator {
				t.Fatalf("WASI emulator = %q, want %q", export.Emulator, WASIThreadedEmulator)
			}
		})
	}
}

func TestAppendEmscriptenLibffiSearchPath(t *testing.T) {
	var export Export
	appendEmscriptenLibffiSearchPath(&export, "", WasmProfileJ32)
	if len(export.LDFLAGS) != 0 {
		t.Fatalf("empty LLGO_ROOT appended %v", export.LDFLAGS)
	}
	appendEmscriptenLibffiSearchPath(&export, "/llgo", WasmProfileJ64)
	want64 := "-L" + filepath.Join("/llgo", wasm64LibffiRelDir)
	if !slices.Contains(export.LDFLAGS, want64) {
		t.Fatalf("memory64 LDFLAGS %v do not search %s", export.LDFLAGS, want64)
	}
	appendEmscriptenLibffiSearchPath(&export, "/llgo", WasmProfileNone)
	want32 := "-L" + filepath.Join("/llgo", wasm32LibffiRelDir)
	if !slices.Contains(export.LDFLAGS, want32) {
		t.Fatalf("unspecified ABI LDFLAGS %v do not search %s", export.LDFLAGS, want32)
	}
	appendEmscriptenLibffiSearchPath(&export, "/llgo", WasmProfileJ32)
	n := 0
	for _, flag := range export.LDFLAGS {
		if flag == want32 {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("emscripten search path count = %d, want 2 in %v", n, export.LDFLAGS)
	}
}

func TestEmscriptenLibffiSearchPath(t *testing.T) {
	root := env.LLGoROOT()
	if root == "" {
		t.Fatal("LLGO_ROOT is required to locate the vendored wasm32 libffi archive")
	}
	libDir := filepath.Join(root, wasm32LibffiRelDir)
	if _, err := os.Stat(filepath.Join(libDir, "libffi.a")); err != nil {
		t.Fatalf("vendored wasm32 libffi archive: %v", err)
	}
	wantL := "-L" + libDir

	js, err := use("js", "wasm", false, optlevel.O2, lto.Off, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(js.LDFLAGS, emscriptenAllowTableGrowth) {
		t.Errorf("raw js/wasm LDFLAGS %v do not allow wasm table growth for libffi closures", js.LDFLAGS)
	}
	if !slices.Contains(js.LDFLAGS, wantL) {
		t.Errorf("raw js/wasm LDFLAGS %v do not search %s", js.LDFLAGS, wantL)
	}

	named, err := Use("", "", "emscripten", false, optlevel.O2, lto.Off, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(named.LDFLAGS, emscriptenAllowTableGrowth) {
		t.Errorf("emscripten LDFLAGS %v do not allow wasm table growth for libffi closures", named.LDFLAGS)
	}
	if !slices.Contains(named.LDFLAGS, wantL) {
		t.Errorf("emscripten LDFLAGS %v do not search %s", named.LDFLAGS, wantL)
	}

	memory64, err := Use("", "", "emscripten-memory64", false, optlevel.O2, lto.Off, false)
	if err != nil {
		t.Fatal(err)
	}
	want64 := "-L" + filepath.Join(root, wasm64LibffiRelDir)
	if slices.Contains(memory64.LDFLAGS, wantL) {
		t.Errorf("emscripten-memory64 LDFLAGS %v unexpectedly search the wasm32 libffi archive", memory64.LDFLAGS)
	}
	if !slices.Contains(memory64.LDFLAGS, want64) {
		t.Errorf("emscripten-memory64 LDFLAGS %v do not search %s", memory64.LDFLAGS, want64)
	}
	if _, err := os.Stat(filepath.Join(root, wasm64LibffiRelDir, "libffi.a")); err != nil {
		t.Fatalf("vendored wasm64 libffi archive: %v", err)
	}

	wasi, err := use("wasip1", "wasm", false, optlevel.O2, lto.Off, false)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(wasi.LDFLAGS, wantL) || slices.Contains(wasi.LDFLAGS, emscriptenAllowTableGrowth) {
		t.Errorf("wasip1/wasm LDFLAGS %v unexpectedly include Emscripten libffi flags", wasi.LDFLAGS)
	}
}

func TestRawWasmProfiles(t *testing.T) {
	js, err := use("js", "wasm", false, optlevel.O2, lto.Off, false)
	if err != nil {
		t.Fatal(err)
	}
	if js.WasmProfile != WasmProfileJ32 || js.WasmProvider != WasmProviderGoJS || js.LLVMTarget != "wasm32-unknown-emscripten" {
		t.Fatalf("raw js/wasm = profile/provider %q/%q, LLVM profile %q", js.WasmProfile, js.WasmProvider, js.LLVMTarget)
	}
	if !slices.Contains(js.LDFLAGS, "-sSTACK_OVERFLOW_CHECK=2") {
		t.Fatalf("raw js/wasm does not check Fiber stack bounds: %v", js.LDFLAGS)
	}
	if !slices.Contains(js.LDFLAGS, "-sENVIRONMENT=web,worker") ||
		slices.Contains(js.LDFLAGS, "-sENVIRONMENT=web,worker,node") {
		t.Fatalf("raw js/wasm host environment unexpectedly changed: %v", js.LDFLAGS)
	}
	if slices.Contains(js.BuildTags, "llgo.wasm.emscripten") {
		t.Fatalf("raw js/wasm acquired an Emscripten source tag: %v", js.BuildTags)
	}

	wasi, err := use("wasip1", "wasm", false, optlevel.O2, lto.Off, false)
	if err != nil {
		t.Fatal(err)
	}
	if wasi.WasmProfile != WasmProfileW32 || wasi.WasmProvider != WasmProviderWASI || wasi.LLVMTarget != "wasm32-unknown-wasip1" {
		t.Fatalf("raw wasip1/wasm = profile/provider %q/%q, LLVM profile %q", wasi.WasmProfile, wasi.WasmProvider, wasi.LLVMTarget)
	}
	if !slices.Contains(wasi.LDFLAGS, "-Wl,--import-memory") {
		t.Fatal("raw WASI threads must import shared memory")
	}
	if wasi.WasmPostLink.Asyncify {
		t.Fatal("raw WASI threads unexpectedly request Asyncify")
	}
	if !slices.Contains(wasi.BuildTags, "llgo.wasi_threads") {
		t.Fatalf("raw WASI threads source tag missing: %v", wasi.BuildTags)
	}
	if !slices.Contains(wasi.BuildTags, "llgo.wasm.wasi") {
		t.Fatalf("raw wasip1/wasm did not select the WASI provider source tag: %v", wasi.BuildTags)
	}
}

func writeWasmTargetFixture(t *testing.T, name, config string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "runtime", "go.mod"),
		[]byte("module github.com/xgo-dev/llgo/runtime\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "targets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if name != "" {
		if err := os.WriteFile(filepath.Join(root, "targets", name+".json"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("LLGO_ROOT", root)
}

func TestNamedWasmTargetUsesInheritedProfile(t *testing.T) {
	writeWasmTargetFixture(t, "hosted-j32", `{
		"llvm-target":"wasm32-unknown-emscripten",
		"cpu":"generic",
		"build-tags":["llgo.wasm.test-provider"],
		"goos":"js",
		"goarch":"wasm",
		"wasm-profile":"j32",
		"wasm-provider":"emscripten"
	}`)
	root := os.Getenv("LLGO_ROOT")
	if err := os.WriteFile(
		filepath.Join(root, "targets", "custom-wasm-alias.json"),
		[]byte(`{"inherits":["hosted-j32"]}`), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	export, err := Use("", "", "custom-wasm-alias", false, optlevel.O2, lto.Off, false)
	if err != nil {
		t.Fatal(err)
	}
	if export.WasmProfile != WasmProfileJ32 || export.WasmProvider != WasmProviderEmscripten || export.CC != "emcc" {
		t.Fatalf("inherited target selected profile/provider %q/%q with compiler %q", export.WasmProfile, export.WasmProvider, export.CC)
	}
	if !slices.Contains(export.BuildTags, "llgo.wasm.test-provider") {
		t.Fatalf("inherited target lost provider tags: %v", export.BuildTags)
	}
}

func TestNamedWASITargetMemory(t *testing.T) {
	writeWasmTargetFixture(t, "base", `{
		"llvm-target":"wasm32-unknown-wasip1",
		"goos":"wasip1","goarch":"wasm","wasm-profile":"w32","wasm-provider":"wasi",
		"wasm-initial-memory":16777216,"wasm-max-memory":134217728,"wasm-stack-size":1048576
	}`)
	root := os.Getenv("LLGO_ROOT")
	// Configuration resolution only needs the SDK directory to exist.
	if err := os.MkdirAll(filepath.Join(root, "crosscompile", "wasi-libc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "targets", "child.json"),
		[]byte(`{"inherits":["base"],"wasm-stack-size":2097152}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"base", "child", ""} {
		export, err := Use("wasip1", "wasm", target, false, optlevel.Os, lto.Off, false)
		if err != nil {
			t.Fatal(err)
		}
		initial, stack, maximum := uint64(16<<20), "stack-size=1048576", "-Wl,--max-memory=134217728"
		if target == "child" {
			stack = "stack-size=2097152"
		} else if target == "" {
			initial, stack, maximum = 0, "stack-size=10485760", "-Wl,--max-memory=536870912"
		}
		if export.WASIInitialMemory != initial || !slices.Contains(export.LDFLAGS, stack) || !slices.Contains(export.LDFLAGS, maximum) {
			t.Fatalf("target %q: initial=%d flags=%v", target, export.WASIInitialMemory, export.LDFLAGS)
		}
		var stacks, maxima int
		for _, flag := range export.LDFLAGS {
			if strings.HasPrefix(flag, "stack-size=") {
				stacks++
			}
			if strings.HasPrefix(flag, "-Wl,--max-memory=") {
				maxima++
			}
		}
		if stacks != 1 || maxima != 1 {
			t.Fatalf("target %q: expected one stack and maximum memory flag, got %d/%d", target, stacks, maxima)
		}
		if !slices.Contains(export.BuildTags, "llgo.wasi_threads") || !slices.Contains(export.LDFLAGS, "-Wl,--import-memory") {
			t.Fatalf("target %q lost WASI threads/shared memory", target)
		}
	}
}

func TestWasmProfileValidationErrors(t *testing.T) {
	for _, test := range []struct {
		name     string
		profile  WasmProfile
		provider WasmProvider
	}{
		{"unknown profile", WasmProfile("invalid"), WasmProviderGoJS},
		{"unknown provider", WasmProfileJ32, WasmProvider("invalid")},
		{"missing profile", WasmProfileNone, WasmProviderGoJS},
		{"J64 GoJS", WasmProfileJ64, WasmProviderGoJS},
		{"W32 Emscripten", WasmProfileW32, WasmProviderEmscripten},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := useWithGOARMAndToolchain(
				"js", "wasm", "", false, optlevel.O2, lto.Off, false,
				test.profile, test.provider,
			)
			if err == nil || !strings.Contains(err.Error(), "unsupported WebAssembly profile/provider") {
				t.Fatalf("invalid direct profile/provider error = %v", err)
			}
		})
	}

	t.Run("generic target", func(t *testing.T) {
		writeWasmTargetFixture(t, "invalid-wasm", `{
			"llvm-target":"wasm32-unknown-unknown",
			"cpu":"generic",
			"goos":"linux",
			"goarch":"wasm",
			"wasm-profile":"invalid",
			"wasm-provider":"gojs"
		}`)
		_, err := Use("", "", "invalid-wasm", false, optlevel.O2, lto.Off, false)
		if err == nil || !strings.Contains(err.Error(), "unsupported WebAssembly profile/provider") {
			t.Fatalf("invalid target profile error = %v", err)
		}
	})

	t.Run("missing named target", func(t *testing.T) {
		writeWasmTargetFixture(t, "", "")
		_, err := Use("", "", "emscripten", false, optlevel.O2, lto.Off, false)
		if err == nil || !strings.Contains(err.Error(), "failed to resolve target emscripten") {
			t.Fatalf("missing named target error = %v", err)
		}
	})

	t.Run("invalid named profile", func(t *testing.T) {
		writeWasmTargetFixture(t, "emscripten", `{
			"llvm-target":"wasm32-unknown-emscripten",
			"goos":"js",
			"goarch":"wasm",
			"wasm-profile":"invalid",
			"wasm-provider":"emscripten"
		}`)
		_, err := Use("", "", "emscripten", false, optlevel.O2, lto.Off, false)
		if err == nil || !strings.Contains(err.Error(), "unsupported WebAssembly profile/provider") {
			t.Fatalf("invalid named profile error = %v", err)
		}
	})

	t.Run("invalid named platform", func(t *testing.T) {
		writeWasmTargetFixture(t, "emscripten", `{
			"llvm-target":"wasm32-unknown-emscripten",
			"goos":"plan9",
			"goarch":"wasm",
			"wasm-profile":"j32",
			"wasm-provider":"emscripten"
		}`)
		_, err := Use("", "", "emscripten", false, optlevel.O2, lto.Off, false)
		if err == nil || !strings.Contains(err.Error(), "unsupported GOOS for WebAssembly") {
			t.Fatalf("invalid named platform error = %v", err)
		}
	})

	t.Run("LLVM target mismatch", func(t *testing.T) {
		writeWasmTargetFixture(t, "emscripten", `{
			"llvm-target":"wasm32-wrong-emscripten",
			"goos":"js",
			"goarch":"wasm",
			"wasm-profile":"j32",
			"wasm-provider":"emscripten"
		}`)
		_, err := Use("", "", "emscripten", false, optlevel.O2, lto.Off, false)
		if err == nil || !strings.Contains(err.Error(), "requires \"wasm32-unknown-emscripten\"") {
			t.Fatalf("mismatched target error = %v", err)
		}
	})
}

func TestUseWithTarget(t *testing.T) {
	// Test target-based configuration takes precedence
	export, err := Use("linux", "amd64", "wasi", true, optlevel.Oz, lto.Thin, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !hasFlagValue(export.CCFLAGS, "-target", "wasm32-wasip1-threads") {
		t.Errorf("Use(linux, amd64, wasi) CCFLAGS = %v, want -target wasm32-wasip1-threads", export.CCFLAGS)
	}

	// Test fallback to goos/goarch when no target specified
	export, err = Use(runtime.GOOS, runtime.GOARCH, "", false, optlevel.O2, lto.Thin, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should use native configuration (only check for macOS since that's where tests run)
	if runtime.GOOS == "darwin" && len(export.LDFLAGS) == 0 {
		t.Error("Expected LDFLAGS to be set for native build")
	}
	wantDebugInfo := nativeDebugInfoPolicy(export.Toolchain)
	if export.DebugInfo.Capability != wantDebugInfo.Capability ||
		!slices.Equal(export.DebugInfo.OmitLinkFlags, wantDebugInfo.OmitLinkFlags) ||
		!slices.Equal(export.DebugInfo.PreserveLinkFlags, wantDebugInfo.PreserveLinkFlags) {
		t.Fatalf("native debug-info policy = %+v, want %+v", export.DebugInfo, wantDebugInfo)
	}
}

func TestNativeToolchain(t *testing.T) {
	tests := []struct {
		goos string
		want NativeToolchain
	}{
		{"darwin", NativeToolchain{ABI: PlatformABIDarwin, ObjectFormat: ObjectFormatMachO, Driver: DriverFlavorClangGNU, Linker: LinkerFlavorMachO}},
		{"linux", NativeToolchain{ABI: PlatformABIGNU, ObjectFormat: ObjectFormatELF, Driver: DriverFlavorClangGNU, Linker: LinkerFlavorELFLLD}},
		{"freebsd", NativeToolchain{}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			if got := nativeToolchain(tt.goos); got != tt.want {
				t.Fatalf("nativeToolchain(%q) = %+v, want %+v", tt.goos, got, tt.want)
			}
		})
	}
}

func TestNativeDebugInfoPolicy(t *testing.T) {
	tests := []struct {
		goos     string
		omit     []string
		preserve []string
	}{
		{goos: "darwin", omit: []string{"-Wl,-S"}},
		{goos: "linux", omit: []string{"-Wl,-S"}},
		{goos: "freebsd"},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			policy := nativeDebugInfoPolicy(nativeToolchain(tt.goos))
			if !policy.CanRetain() || !slices.Equal(policy.OmitLinkFlags, tt.omit) || !slices.Equal(policy.PreserveLinkFlags, tt.preserve) {
				t.Fatalf("nativeDebugInfoPolicy(%q) = %+v, want omit=%v preserve=%v", tt.goos, policy, tt.omit, tt.preserve)
			}
			if !slices.Equal(policy.PreserveDriverFlags, []string{"-gdwarf-4"}) {
				t.Fatalf("native compiler-driver DWARF flags = %v, want -gdwarf-4", policy.PreserveDriverFlags)
			}
		})
	}
}

func TestOptimizationFlagPlacement(t *testing.T) {
	export, err := Use("", "", "wasi", false, optlevel.Oz, lto.Off, false)
	if err != nil {
		t.Fatalf("Use(wasi, Oz) failed: %v", err)
	}
	if len(export.CCFLAGS) == 0 || export.CCFLAGS[0] != "-Oz" {
		t.Fatalf("target CCFLAGS = %v, want first flag -Oz", export.CCFLAGS)
	}

	export, err = Use(runtime.GOOS, runtime.GOARCH, "", false, optlevel.O3, lto.Off, false)
	if err != nil {
		t.Fatalf("UseWithOptLevel(host, O3) failed: %v", err)
	}
	if !slices.Contains(export.CCFLAGS, "-O3") {
		t.Fatalf("host CCFLAGS = %v, want -O3", export.CCFLAGS)
	}
	wantTarget := llvm.GetTargetTriple(runtime.GOOS, runtime.GOARCH)
	if export.Toolchain.TargetTriple != "" {
		wantTarget = export.Toolchain.TargetTriple
	}
	if !hasFlagValue(export.CCFLAGS, "-target", wantTarget) {
		t.Fatalf("host CCFLAGS = %v, want native -target", export.CCFLAGS)
	}
	if !hasFlagValue(export.LDFLAGS, "-target", wantTarget) {
		t.Fatalf("host LDFLAGS = %v, want native -target", export.LDFLAGS)
	}
}

func TestLTOLinkerOptFlag(t *testing.T) {
	tests := []struct {
		level optlevel.Level
		want  string
	}{
		{level: optlevel.O0, want: "--lto-O0"},
		{level: optlevel.O1, want: "--lto-O1"},
		{level: optlevel.O2, want: "--lto-O2"},
		{level: optlevel.O3, want: "--lto-O3"},
		{level: optlevel.Os, want: "--lto-O2"},
		{level: optlevel.Oz, want: "--lto-O2"},
		{level: optlevel.Unset, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			if got := ltoLinkerOptFlag(tt.level); got != tt.want {
				t.Fatalf("ltoLinkerOptFlag(%v) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestUsesNativePlatformToolchain(t *testing.T) {
	for _, test := range []struct {
		name                                   string
		hostOS, hostArch, targetOS, targetArch string
		want                                   bool
	}{
		{name: "same platform", hostOS: "linux", hostArch: "amd64", targetOS: "linux", targetArch: "amd64", want: true},
		{name: "cross architecture", hostOS: "darwin", hostArch: "arm64", targetOS: "darwin", targetArch: "amd64"},
		{name: "cross OS", hostOS: "linux", hostArch: "amd64", targetOS: "darwin", targetArch: "amd64"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := usesNativePlatformToolchain(test.hostOS, test.hostArch, test.targetOS, test.targetArch); got != test.want {
				t.Fatalf("usesNativePlatformToolchain() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDevLTOGlobalDCEUseLTOFlagsControlledByOption(t *testing.T) {
	export, err := use(runtime.GOOS, runtime.GOARCH, false, optlevel.O2, lto.Off, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	for _, flag := range export.CCFLAGS {
		if strings.HasPrefix(flag, "-flto") {
			t.Fatalf("unexpected LTO ccflag when disabled: %q", flag)
		}
	}
	for _, flag := range export.LDFLAGS {
		if strings.Contains(flag, "lto-") {
			t.Fatalf("unexpected LTO ldflag when disabled: %q", flag)
		}
	}

	thin, err := use(runtime.GOOS, runtime.GOARCH, false, optlevel.O2, lto.Thin, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !slices.Contains(thin.CCFLAGS, "-flto=thin") {
		t.Fatalf("missing thin LTO ccflag: %v", thin.CCFLAGS)
	}
	if slices.Contains(thin.CCFLAGS, "-fvirtual-function-elimination") {
		t.Fatalf("unexpected virtual function elimination ccflag for thin LTO: %v", thin.CCFLAGS)
	}
	if slices.Contains(thin.CCFLAGS, "-fwhole-program-vtables") {
		t.Fatalf("unexpected whole-program vtables ccflag for thin LTO: %v", thin.CCFLAGS)
	}
	if !slices.Contains(thin.LDFLAGS, "-flto=thin") {
		t.Fatalf("missing thin LTO link driver flag: %v", thin.LDFLAGS)
	}
	wantLTOOpt := nativeLTOOptFlag(thin.Toolchain, optlevel.O2)
	if !slices.Contains(thin.LDFLAGS, wantLTOOpt) {
		t.Fatalf("missing thin LTO linker opt flag: %v", thin.LDFLAGS)
	}

	thinSize, err := use(runtime.GOOS, runtime.GOARCH, false, optlevel.Oz, lto.Thin, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !slices.Contains(thinSize.LDFLAGS, nativeLTOOptFlag(thinSize.Toolchain, optlevel.Oz)) {
		t.Fatalf("missing numeric thin LTO linker opt flag for Oz: %v", thinSize.LDFLAGS)
	}
	if slices.Contains(thinSize.LDFLAGS, "-Wl,--lto-Oz") || slices.Contains(thinSize.LDFLAGS, "-Wl,/opt:lldlto=Oz") {
		t.Fatalf("invalid size-valued thin LTO linker opt flag: %v", thinSize.LDFLAGS)
	}

	full, err := use(runtime.GOOS, runtime.GOARCH, false, optlevel.O2, lto.Full, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !slices.Contains(full.CCFLAGS, "-flto=full") {
		t.Fatalf("missing full LTO ccflag: %v", full.CCFLAGS)
	}
	if slices.Contains(full.CCFLAGS, "-fvirtual-function-elimination") {
		t.Fatalf("unexpected virtual function elimination ccflag when global DCE is disabled: %v", full.CCFLAGS)
	}
	if slices.Contains(full.CCFLAGS, "-fwhole-program-vtables") {
		t.Fatalf("unexpected whole-program vtables ccflag when global DCE is disabled: %v", full.CCFLAGS)
	}
	if !slices.Contains(full.LDFLAGS, "-flto=full") {
		t.Fatalf("missing full LTO link driver flag: %v", full.LDFLAGS)
	}

	fullGlobalDCE, err := use(runtime.GOOS, runtime.GOARCH, false, optlevel.O2, lto.Full, true)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !slices.Contains(fullGlobalDCE.CCFLAGS, "-fvirtual-function-elimination") {
		t.Fatalf("missing virtual function elimination ccflag for full LTO with global DCE: %v", fullGlobalDCE.CCFLAGS)
	}
	if !slices.Contains(fullGlobalDCE.CCFLAGS, "-fwhole-program-vtables") {
		t.Fatalf("missing whole-program vtables ccflag for full LTO with global DCE: %v", fullGlobalDCE.CCFLAGS)
	}
}

func nativeLTOOptFlag(toolchain NativeToolchain, level optlevel.Level) string {
	return "-Wl," + ltoLinkerOptFlag(level)
}

func hasMllvmOption(flags []string, opt string) bool {
	for i := 0; i+1 < len(flags); i++ {
		if flags[i] == "-mllvm" && flags[i+1] == opt {
			return true
		}
	}
	return false
}

func hasFlagValue(flags []string, flag, value string) bool {
	for i := 0; i+1 < len(flags); i++ {
		if flags[i] == flag && flags[i+1] == value {
			return true
		}
	}
	return false
}

func TestEmscriptenLTOFlags(t *testing.T) {
	for _, target := range []string{"", "emscripten", "emscripten-memory64"} {
		for _, mode := range []lto.Mode{lto.Off, lto.Thin, lto.Full} {
			for _, level := range []optlevel.Level{optlevel.O0, optlevel.O1, optlevel.O2, optlevel.O3, optlevel.Os, optlevel.Oz} {
				t.Run(target+"/"+mode.String()+"/"+level.Name(), func(t *testing.T) {
					export, err := Use("js", "wasm", target, false, level, mode, false)
					if err != nil {
						t.Fatal(err)
					}
					for name, flags := range map[string][]string{"compile": export.CCFLAGS, "link": export.LDFLAGS} {
						if !slices.Contains(flags, level.Flag()) {
							t.Errorf("%s flags do not preserve %s: %v", name, level, flags)
						}
						for _, candidate := range []lto.Mode{lto.Thin, lto.Full} {
							if slices.Contains(flags, candidate.ClangFlag()) != (mode == candidate) {
								t.Errorf("%s flags for %s: incorrect %s in %v", name, mode, candidate.ClangFlag(), flags)
							}
						}
					}
					var got []string
					for _, flag := range export.LDFLAGS {
						if strings.HasPrefix(flag, "-Wl,--lto-O") {
							got = append(got, flag)
						}
					}
					var want []string
					if mode.Enabled() {
						want = []string{"-Wl," + ltoLinkerOptFlag(level)}
					}
					if !slices.Equal(got, want) {
						t.Errorf("LTO optimizer flags = %v, want %v", got, want)
					}
				})
			}
		}
	}
}
