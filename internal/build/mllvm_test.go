package build

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/xgo-dev/llgo/internal/crosscompile"
	"github.com/xgo-dev/llgo/internal/lto"
	"github.com/xgo-dev/llgo/internal/packages"
)

func TestCollectFingerprintIncludesMLLVM(t *testing.T) {
	goFile := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(goFile, []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	collect := func(opts []string) *aPackage {
		pkg := &aPackage{Package: &packages.Package{PkgPath: "test/pkg", GoFiles: []string{goFile}}}
		ctx := &context{
			conf:         &packages.Config{},
			buildConf:    &Config{Goos: "linux", Goarch: "amd64", MLLVM: opts},
			crossCompile: crosscompile.Export{LLVMTarget: "x86_64-unknown-linux"},
		}
		if err := ctx.collectFingerprint(pkg); err != nil {
			t.Fatal(err)
		}
		return pkg
	}

	without := collect(nil)
	align6 := collect([]string{"-align-all-functions=6"})
	align5 := collect([]string{"-align-all-functions=5"})
	if without.Fingerprint == align6.Fingerprint {
		t.Error("builds with and without -mllvm share a cache fingerprint")
	}
	if align6.Fingerprint == align5.Fingerprint {
		t.Error("builds with different -mllvm options share a cache fingerprint")
	}
	data, err := decodeManifest(align6.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"-align-all-functions=6"}; data.Common == nil || !reflect.DeepEqual(data.Common.MLLVM, want) {
		t.Errorf("manifest MLLVM missing %q:\n%s", want, align6.Manifest)
	}
}

func TestMLLVMLinkerArgs(t *testing.T) {
	opts := []string{"-align-all-functions=6", "-x86-branches-within-32B-boundaries"}
	viaLinker := []string{
		"-Xlinker", "-mllvm", "-Xlinker", "-align-all-functions=6",
		"-Xlinker", "-mllvm", "-Xlinker", "-x86-branches-within-32B-boundaries",
	}
	tests := []struct {
		name string
		conf Config
		want []string
	}{
		{name: "no-options", conf: Config{LTO: lto.Thin}, want: nil},
		{name: "no-lto", conf: Config{MLLVM: opts}, want: nil},
		{name: "thin", conf: Config{LTO: lto.Thin, MLLVM: opts}, want: viaLinker},
		{name: "full", conf: Config{LTO: lto.Full, MLLVM: opts}, want: viaLinker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mllvmLinkerArgs(&tt.conf); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mllvmLinkerArgs(LTO=%v, MLLVM=%q) = %q, want %q", tt.conf.LTO, tt.conf.MLLVM, got, tt.want)
			}
		})
	}
}

func TestApplyMLLVMRejectsUnknownOption(t *testing.T) {
	err := applyMLLVM([]string{"-owngo-no-such-option"})
	if err == nil || !strings.Contains(err.Error(), "owngo-no-such-option") {
		t.Errorf("applyMLLVM(-owngo-no-such-option) error = %v, want an LLVM error naming the option", err)
	}
}

const mllvmHelperEnv = "LLGO_TEST_MLLVM_BUILD"

// TestMLLVMAlignsFunctions збирає програму в дочірньому процесі: опції LLVM
// глобальні для процесу і після розбору лишились би в інших тестах пакета.
func TestMLLVMAlignsFunctions(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("checks ELF symbol addresses")
	}
	if testing.Short() {
		t.Skip("builds three executables")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(src, []byte(mllvmProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		lto     string
		opts    string
		aligned bool
	}{
		{name: "default", aligned: false},
		{name: "no-lto", opts: "-align-all-functions=6", aligned: true},
		{name: "thin", lto: "thin", opts: "-align-all-functions=6", aligned: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "mllvm")
			cmd := exec.Command(os.Args[0], "-test.run=^TestMLLVMBuildHelper$")
			cmd.Env = append(os.Environ(),
				"LLGO_ROOT="+root,
				"XDG_CACHE_HOME="+t.TempDir(),
				mllvmHelperEnv+"="+strings.Join([]string{src, bin, tt.lto, tt.opts}, "\n"),
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build helper: %v\n%s", err, out)
			}
			addrs := mainFuncAddrs(t, bin)
			misaligned := 0
			for name, addr := range addrs {
				if addr%64 != 0 {
					misaligned++
					if tt.aligned {
						t.Errorf("%s at %#x, want a multiple of 64", name, addr)
					}
				}
			}
			if !tt.aligned && misaligned == 0 {
				t.Errorf("all %d main functions are 64-byte aligned without -mllvm", len(addrs))
			}
		})
	}
}

func TestMLLVMBuildHelper(t *testing.T) {
	spec := os.Getenv(mllvmHelperEnv)
	if spec == "" {
		t.Skip("helper for TestMLLVMAlignsFunctions")
	}
	parts := strings.Split(spec, "\n")
	src, bin, ltoName, opts := parts[0], parts[1], parts[2], parts[3]
	conf := &Config{Mode: ModeBuild, BuildMode: BuildModeExe, OutFile: bin}
	if opts != "" {
		conf.MLLVM = []string{opts}
	}
	if ltoName != "" {
		mode, err := lto.Parse(ltoName)
		if err != nil {
			t.Fatal(err)
		}
		conf.LTO = mode
	}
	if _, err := Do([]string{src}, conf); err != nil {
		t.Fatal(err)
	}
}

// mainFuncAddrs повертає адреси функцій пакета main з таблиці символів ELF.
func mainFuncAddrs(t *testing.T, bin string) map[string]uint64 {
	t.Helper()
	f, err := elf.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	syms, err := f.Symbols()
	if err != nil {
		t.Fatal(err)
	}
	addrs := make(map[string]uint64)
	for _, s := range syms {
		if elf.ST_TYPE(s.Info) == elf.STT_FUNC && strings.HasPrefix(s.Name, "main.") && s.Value != 0 {
			addrs[s.Name] = s.Value
		}
	}
	if len(addrs) < 4 {
		t.Fatalf("%s: found main functions %v, want at least 4", bin, addrs)
	}
	return addrs
}

const mllvmProgram = `package main

import "os"

//go:noinline
func a(x int) int { return x + 1 }

//go:noinline
func b(x int) int { return x * 3 }

//go:noinline
func c(x int) int { return x - 7 }

//go:noinline
func d(x int) int { return x ^ 5 }

//go:noinline
func e(x int) int { return x << 2 }

func main() {
	os.Exit(a(b(c(d(e(len(os.Args)))))) & 0)
}
`
