package build

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestApplyGopacabraTag(t *testing.T) {
	tests := []struct {
		name   string
		tags   string
		asan   bool
		goos   string
		goarch string
		target string
		mode   BuildMode
		want   string
	}{
		// gopacabra — єдина збірка: BDWGC видалено, тег додається завжди.
		{name: "empty", tags: "", want: "gopacabra,nogc,gopacabramn"},
		{name: "other", tags: "purego", want: "purego,gopacabra,nogc,gopacabramn"},
		{name: "nogopacabra is not an opt-out", tags: "nogopacabra", want: "nogopacabra,gopacabra,nogc,gopacabramn"},
		// Звіт на виході — лише на запит; gopacabrastat його вмикає.
		{name: "gopacabrastat reports", tags: "gopacabrastat", want: "gopacabrastat,gopacabra,nogc,gopacabramn,gopacabrareport"},
		{name: "gopacabrareport given", tags: "gopacabrareport", want: "gopacabrareport,gopacabra,nogc,gopacabramn"},
		{name: "empty asan", tags: "", asan: true, want: "gopacabra,nogc,gopacabramn,gopacabraasan"},
		{name: "gopacabra", tags: "gopacabra", want: "gopacabra,nogc,gopacabramn"},
		{name: "gopacabra among others", tags: "foo,gopacabra,bar", want: "foo,gopacabra,bar,nogc,gopacabramn"},
		{name: "already nogc", tags: "gopacabra,nogc", want: "gopacabra,nogc,gopacabramn"},
		{name: "nogc before gopacabra", tags: "nogc,gopacabra", want: "nogc,gopacabra,gopacabramn"},
		{name: "gopacabramn given", tags: "gopacabra,gopacabramn", want: "gopacabra,gopacabramn,nogc"},
		{name: "gopacabra asan", tags: "gopacabra", asan: true, want: "gopacabra,nogc,gopacabramn,gopacabraasan"},
		{name: "gopacabraasan given", tags: "gopacabra,nogc,gopacabramn,gopacabraasan", asan: true, want: "gopacabra,nogc,gopacabramn,gopacabraasan"},
		{name: "1:1", tags: "gopacabra,gopacabra1to1", want: "gopacabra,gopacabra1to1,nogc"},
		{name: "1:1 asan", tags: "gopacabra,gopacabra1to1", asan: true, want: "gopacabra,gopacabra1to1,nogc,gopacabraasan"},
		{name: "1:1 with explicit gopacabramn", tags: "gopacabra,gopacabra1to1,gopacabramn", want: "gopacabra,gopacabra1to1,gopacabramn,nogc"},
		{name: "darwin arm64", tags: "gopacabra", goos: "darwin", goarch: "arm64", want: "gopacabra,nogc,gopacabramn"},
		{name: "linux arm64", tags: "gopacabra", goos: "linux", goarch: "arm64", want: "gopacabra,nogc,gopacabramn"},
		{name: "linux 386", tags: "gopacabra", goos: "linux", goarch: "386", want: "gopacabra,nogc"},
		{name: "windows", tags: "gopacabra", goos: "windows", goarch: "amd64", want: "gopacabra,nogc"},
		{name: "wasi", tags: "gopacabra", goos: "wasip1", goarch: "wasm", want: "gopacabra,nogc"},
		{name: "js", tags: "gopacabra", goos: "js", goarch: "wasm", want: "gopacabra,nogc"},
		{name: "named target", tags: "gopacabra", target: "wasi", want: "gopacabra,nogc"},
		{name: "named target default", tags: "", target: "wasi", want: "gopacabra,nogc"},
		// M:N запускає планувальник з точки входу exe; бібліотека main не
		// володіє, її експорти йдуть на потоках хоста — модель 1:1.
		{name: "exe", tags: "", mode: BuildModeExe, want: "gopacabra,nogc,gopacabramn"},
		{name: "c-shared", tags: "", mode: BuildModeCShared, want: "gopacabra,nogc"},
		{name: "c-archive", tags: "", mode: BuildModeCArchive, want: "gopacabra,nogc"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			conf := &Config{Tags: test.tags, ASan: test.asan, Goos: test.goos, Goarch: test.goarch, Target: test.target, BuildMode: test.mode}
			if conf.Goos == "" {
				conf.Goos, conf.Goarch = "linux", "amd64"
			}
			applyGopacabraTag(conf)
			if conf.Tags != test.want {
				t.Errorf("applyGopacabraTag(%q, asan %v, %s/%s, target %q, mode %q) = %q, want %q",
					test.tags, test.asan, conf.Goos, conf.Goarch, test.target, test.mode, conf.Tags, test.want)
			}
		})
	}
}

func TestResolveBuildConfigGopacabraDefault(t *testing.T) {
	t.Setenv("GOAMD64", "")
	tests := []struct {
		tags        string
		wantTags    string
		wantGOAMD64 string
	}{
		{tags: "", wantTags: "gopacabra,nogc,gopacabramn", wantGOAMD64: "v3"},
		{tags: "purego", wantTags: "purego,gopacabra,nogc,gopacabramn", wantGOAMD64: "v3"},
	}
	for _, test := range tests {
		conf, err := resolveBuildConfig(&Config{Tags: test.tags, Goos: "linux", Goarch: "amd64"})
		if err != nil {
			t.Fatalf("resolveBuildConfig(tags %q) error = %v", test.tags, err)
		}
		if conf.Tags != test.wantTags || conf.GOAMD64 != test.wantGOAMD64 {
			t.Errorf("resolveBuildConfig(tags %q) = tags %q, GOAMD64 %q, want %q, %q",
				test.tags, conf.Tags, conf.GOAMD64, test.wantTags, test.wantGOAMD64)
		}
	}
}

func TestASanRequiresNativeHost(t *testing.T) {
	conf, err := resolveBuildConfig(&Config{ASan: true})
	if err != nil {
		t.Fatalf("resolveBuildConfig(host asan) error = %v", err)
	}
	if !conf.ASan {
		t.Fatal("resolveBuildConfig(host asan) ASan = false")
	}
	if _, err := resolveBuildConfig(&Config{ASan: true, Target: "wasi"}); err == nil {
		t.Fatal("resolveBuildConfig(asan, wasi) error = nil")
	}
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		return
	}
	if _, err := resolveBuildConfig(&Config{ASan: true, Goos: "linux", Goarch: "amd64"}); err == nil {
		t.Fatal("resolveBuildConfig(asan cross) error = nil")
	}
}

// Горутина з експортованої функції c-shared: бібліотеку завантажує й кличе
// програма на C, точки входу llgo в процесі немає.
func TestGopacabraCSharedGoroutine(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("native ELF/Mach-O shared library")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	src := "package main\n\nimport \"C\"\n\n//export Answer\nfunc Answer() int32 {\n\tch := make(chan int32)\n\tgo func() { ch <- 40 }()\n\treturn <-ch + 2\n}\n\nfunc main() {}\n"
	if err := os.WriteFile(file, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(dir, "libanswer.so")
	if _, err := Do([]string{"file=" + file}, &Config{Mode: ModeBuild, BuildMode: BuildModeCShared, OutFile: lib}); err != nil {
		t.Fatal(err)
	}
	loader := filepath.Join(dir, "loader.c")
	const loaderSrc = `#include <dlfcn.h>
#include <stdio.h>
int main(int argc, char **argv) {
  void *lib = dlopen(argv[1], RTLD_NOW);
  if (!lib) { puts(dlerror()); return 1; }
  int (*answer)(void) = (int (*)(void))dlsym(lib, "Answer");
  if (!answer) return 2;
  printf("%d\n", answer());
  return 0;
}
`
	if err := os.WriteFile(loader, []byte(loaderSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "loader")
	args := []string{loader, "-o", app}
	if runtime.GOOS == "linux" {
		args = append(args, "-ldl")
	}
	if out, err := exec.CommandContext(t.Context(), "cc", args...).CombinedOutput(); err != nil {
		t.Fatalf("cc loader: %v\n%s", err, out)
	}
	out, err := exec.CommandContext(t.Context(), app, lib).CombinedOutput()
	if err != nil || string(out) != "42\n" {
		t.Fatalf("loader(libanswer.so) = %q, %v, want \"42\\n\"", out, err)
	}
}

// TestGopacabraMNRuntimeFreeProgram: під gopacabramn програма, якій рантайм не
// потрібен, не тягне ні планувальника, ні опитувань safepoint; програма з
// горутиною лишається на M:N.
func TestGopacabraMNRuntimeFreeProgram(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("gopacabramn: native linux and darwin")
	}
	tests := []struct {
		name        string
		src         string
		want        string
		wantRuntime bool
	}{
		{"pure-c", pclntabPureCLibraryFixture, "pure-c-pclntab\n", false},
		{"goroutine", "package main\n\nfunc main() {\n\tch := make(chan int)\n\tgo func() { ch <- 1 }()\n\tprintln(<-ch)\n}\n", "1\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "app")
			if _, err := Do([]string{writePCLNIntegrationSource(t, tt.src)}, &Config{Mode: ModeBuild, OutFile: bin}); err != nil {
				t.Fatal(err)
			}
			out, err := exec.CommandContext(t.Context(), bin).CombinedOutput()
			if err != nil || string(out) != tt.want {
				t.Fatalf("%s = %q, %v, want %q", tt.name, out, err, tt.want)
			}
			if got := pclnIntegrationHasLLGoRuntime(t, bin); got != tt.wantRuntime {
				t.Errorf("linked LLGo runtime = %v, want %v", got, tt.wantRuntime)
			}
		})
	}
}

// TestRuntimeLinksNoTracingGC стереже P11.5.0.1: рантайм не імпортує
// обгортку BDWGC і не просить лінкувати libgc, тож збірка не залежить від
// встановленого bdw-gc.
func TestRuntimeLinksNoTracingGC(t *testing.T) {
	const bdwgcPkg = "github.com/xgo-dev/llgo/runtime/internal/clite/bdwgc"
	root := filepath.Join("..", "..", "runtime")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			if p, _ := strconv.Unquote(spec.Path.Value); p == bdwgcPkg {
				t.Errorf("%s imports %s", path, bdwgcPkg)
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, name := range spec.Names {
				if name.Name != "LLGoPackage" || i >= len(spec.Values) {
					continue
				}
				lit, ok := spec.Values[i].(*ast.BasicLit)
				if !ok {
					continue
				}
				v, _ := strconv.Unquote(lit.Value)
				if strings.Contains(v, "bdw-gc") || slices.Contains(strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == ';' }), "-lgc") {
					t.Errorf("%s: LLGoPackage = %q links libgc", path, v)
				}
			}
			return false
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}
