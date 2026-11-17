package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestApplyOwngoTag(t *testing.T) {
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
		// owngo — єдина збірка: BDWGC видалено, тег додається завжди.
		{name: "empty", tags: "", want: "owngo,nogc,owngomn"},
		{name: "other", tags: "purego", want: "purego,owngo,nogc,owngomn"},
		{name: "noowngo is not an opt-out", tags: "noowngo", want: "noowngo,owngo,nogc,owngomn"},
		// Звіт на виході — лише на запит; owngostat його вмикає.
		{name: "owngostat reports", tags: "owngostat", want: "owngostat,owngo,nogc,owngomn,owngoreport"},
		{name: "owngoreport given", tags: "owngoreport", want: "owngoreport,owngo,nogc,owngomn"},
		{name: "empty asan", tags: "", asan: true, want: "owngo,nogc,owngomn,owngoasan"},
		{name: "owngo", tags: "owngo", want: "owngo,nogc,owngomn"},
		{name: "owngo among others", tags: "foo,owngo,bar", want: "foo,owngo,bar,nogc,owngomn"},
		{name: "already nogc", tags: "owngo,nogc", want: "owngo,nogc,owngomn"},
		{name: "nogc before owngo", tags: "nogc,owngo", want: "nogc,owngo,owngomn"},
		{name: "owngomn given", tags: "owngo,owngomn", want: "owngo,owngomn,nogc"},
		{name: "owngo asan", tags: "owngo", asan: true, want: "owngo,nogc,owngomn,owngoasan"},
		{name: "owngoasan given", tags: "owngo,nogc,owngomn,owngoasan", asan: true, want: "owngo,nogc,owngomn,owngoasan"},
		{name: "1:1", tags: "owngo,owngo1to1", want: "owngo,owngo1to1,nogc"},
		{name: "1:1 asan", tags: "owngo,owngo1to1", asan: true, want: "owngo,owngo1to1,nogc,owngoasan"},
		{name: "1:1 with explicit owngomn", tags: "owngo,owngo1to1,owngomn", want: "owngo,owngo1to1,owngomn,nogc"},
		{name: "darwin arm64", tags: "owngo", goos: "darwin", goarch: "arm64", want: "owngo,nogc,owngomn"},
		{name: "linux arm64", tags: "owngo", goos: "linux", goarch: "arm64", want: "owngo,nogc,owngomn"},
		{name: "linux 386", tags: "owngo", goos: "linux", goarch: "386", want: "owngo,nogc"},
		{name: "windows", tags: "owngo", goos: "windows", goarch: "amd64", want: "owngo,nogc"},
		{name: "wasi", tags: "owngo", goos: "wasip1", goarch: "wasm", want: "owngo,nogc"},
		{name: "js", tags: "owngo", goos: "js", goarch: "wasm", want: "owngo,nogc"},
		{name: "named target", tags: "owngo", target: "wasi", want: "owngo,nogc"},
		{name: "named target default", tags: "", target: "wasi", want: "owngo,nogc"},
		// M:N запускає планувальник з точки входу exe; бібліотека main не
		// володіє, її експорти йдуть на потоках хоста — модель 1:1.
		{name: "exe", tags: "", mode: BuildModeExe, want: "owngo,nogc,owngomn"},
		{name: "c-shared", tags: "", mode: BuildModeCShared, want: "owngo,nogc"},
		{name: "c-archive", tags: "", mode: BuildModeCArchive, want: "owngo,nogc"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			conf := &Config{Tags: test.tags, ASan: test.asan, Goos: test.goos, Goarch: test.goarch, Target: test.target, BuildMode: test.mode}
			if conf.Goos == "" {
				conf.Goos, conf.Goarch = "linux", "amd64"
			}
			applyOwngoTag(conf)
			if conf.Tags != test.want {
				t.Errorf("applyOwngoTag(%q, asan %v, %s/%s, target %q, mode %q) = %q, want %q",
					test.tags, test.asan, conf.Goos, conf.Goarch, test.target, test.mode, conf.Tags, test.want)
			}
		})
	}
}

func TestResolveBuildConfigOwngoDefault(t *testing.T) {
	t.Setenv("GOAMD64", "")
	tests := []struct {
		tags        string
		wantTags    string
		wantGOAMD64 string
	}{
		{tags: "", wantTags: "owngo,nogc,owngomn", wantGOAMD64: "v3"},
		{tags: "purego", wantTags: "purego,owngo,nogc,owngomn", wantGOAMD64: "v3"},
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
func TestOwnGoCSharedGoroutine(t *testing.T) {
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
