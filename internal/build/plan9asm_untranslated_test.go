package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Функція .s, яку plan9asm не перекладає, не зупиняє збірку пакета: вона
// лишається без тіла, а посилання на неї падає на компоновці з причиною.
func TestPlan9AsmUntranslatedFuncFailsOnlyWhenReferenced(t *testing.T) {
	var asm string
	switch runtime.GOARCH {
	case "amd64":
		asm = `#include "textflag.h"
TEXT ·Add(SB), NOSPLIT, $0-24
	MOVQ a+0(FP), AX
	ADDQ b+8(FP), AX
	MOVQ AX, ret+16(FP)
	RET
TEXT ·Sum(SB), NOSPLIT, $0-24
	MOVQ p_Lo+0(FP), AX
	ADDQ p_Hi+8(FP), AX
	MOVQ AX, ret+16(FP)
	RET
`
	case "arm64":
		asm = `#include "textflag.h"
TEXT ·Add(SB), NOSPLIT, $0-24
	MOVD a+0(FP), R0
	MOVD b+8(FP), R1
	ADD R1, R0
	MOVD R0, ret+16(FP)
	RET
TEXT ·Sum(SB), NOSPLIT, $0-24
	MOVD p_Lo+0(FP), R0
	MOVD p_Hi+8(FP), R1
	ADD R1, R0
	MOVD R0, ret+16(FP)
	RET
`
	default:
		t.Skip("Plan 9 asm fixture is written for amd64 and arm64")
	}
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/p9untranslated\n\ngo 1.21\n",
		"asmlib/asmlib.go": `package asmlib

type Pair struct{ Lo, Hi uint64 }

func Add(a, b int64) int64
func Sum(p Pair) uint64
`,
		"asmlib/asmlib_" + runtime.GOARCH + ".s": asm,
		"ok/main.go": `package main

import "example.com/p9untranslated/asmlib"

func main() { println(asmlib.Add(2, 3)) }
`,
		"bad/main.go": `package main

import "example.com/p9untranslated/asmlib"

func main() { println(asmlib.Sum(asmlib.Pair{Lo: 2, Hi: 3})) }
`,
	}
	for name, s := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)

	t.Run("unreferenced", func(t *testing.T) {
		conf := NewDefaultConf(ModeBuild)
		conf.OutFile = filepath.Join(dir, "ok.bin")
		if _, err := Do([]string{"./ok"}, conf); err != nil {
			t.Fatalf("Do(./ok) error = %v, want nil", err)
		}
		out, err := exec.CommandContext(t.Context(), conf.OutFile).CombinedOutput() //nolint:gosec // G204: runs the binary this test just built
		if err != nil || string(out) != "5\n" {
			t.Fatalf("run ./ok: %v, output %q, want %q", err, out, "5\n")
		}
	})
	t.Run("referenced", func(t *testing.T) {
		conf := NewDefaultConf(ModeBuild)
		conf.OutFile = filepath.Join(dir, "bad.bin")
		_, err := Do([]string{"./bad"}, conf)
		if err == nil {
			t.Fatal("Do(./bad) error = nil, want link error for untranslated asmlib.Sum")
		}
		for _, want := range []string{"example.com/p9untranslated/asmlib.Sum", "unsupported struct type"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Do(./bad) error = %q, want it to contain %q", err, want)
			}
		}
	})
}
