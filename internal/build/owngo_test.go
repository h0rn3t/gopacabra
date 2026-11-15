package build

import (
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
		want   string
	}{
		{name: "empty", tags: "", want: ""},
		{name: "other", tags: "purego", want: "purego"},
		{name: "owngo", tags: "owngo", want: "owngo,nogc,owngomn"},
		{name: "owngo among others", tags: "foo,owngo,bar", want: "foo,owngo,bar,nogc,owngomn"},
		{name: "already nogc", tags: "owngo,nogc", want: "owngo,nogc,owngomn"},
		{name: "nogc before owngo", tags: "nogc,owngo", want: "nogc,owngo,owngomn"},
		{name: "owngomn given", tags: "owngo,owngomn", want: "owngo,owngomn,nogc"},
		{name: "owngo asan", tags: "owngo", asan: true, want: "owngo,nogc,owngomn,owngoasan"},
		{name: "asan without owngo", tags: "purego", asan: true, want: "purego"},
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			conf := &Config{Tags: test.tags, ASan: test.asan, Goos: test.goos, Goarch: test.goarch, Target: test.target}
			if conf.Goos == "" {
				conf.Goos, conf.Goarch = "linux", "amd64"
			}
			applyOwngoTag(conf)
			if conf.Tags != test.want {
				t.Errorf("applyOwngoTag(%q, asan %v, %s/%s, target %q) = %q, want %q",
					test.tags, test.asan, conf.Goos, conf.Goarch, test.target, conf.Tags, test.want)
			}
		})
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
