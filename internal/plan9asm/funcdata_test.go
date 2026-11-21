package plan9asm

import (
	"strings"
	"testing"
)

// Макроси funcdata.h не генерують коду, тож перекладач їх просто пропускає.
func TestTranslateFuncdataMacros(t *testing.T) {
	pkg := mustTestPackage(t, "example.com/fd", "package fd\nfunc F(a, b int64) int64\nfunc add(a, b int64) int64 { return a + b }\n")
	tests := []struct {
		goarch   string
		asm      string
		wantCall bool
	}{
		{
			goarch: "amd64",
			asm: `#include "funcdata.h"
#include "textflag.h"
TEXT ·F(SB),$24-24
	GO_ARGS
	NO_LOCAL_POINTERS
	MOVQ $0, ret+16(FP)
	GO_RESULTS_INITIALIZED
	MOVQ a+0(FP), AX
	MOVQ AX, 0(SP)
	MOVQ b+8(FP), AX
	MOVQ AX, 8(SP)
	CALL ·add(SB)
	MOVQ 16(SP), AX
	MOVQ AX, ret+16(FP)
	RET
`,
			wantCall: true,
		},
		{
			goarch: "arm64",
			asm: `#include "funcdata.h"
TEXT ·F(SB),$0-24
	GO_ARGS
	NO_LOCAL_POINTERS
	GO_RESULTS_INITIALIZED
	MOVD a+0(FP), R0
	MOVD R0, ret+16(FP)
	RET
`,
		},
	}
	for _, test := range tests {
		t.Run(test.goarch, func(t *testing.T) {
			tr, err := TranslateSourceModuleForPkgWithOptions(pkg, "fd_"+test.goarch+".s", []byte(test.asm), "linux", test.goarch, TranslateOptions{GOAMD64: "v3"})
			if err != nil {
				t.Fatalf("TranslateSourceModuleForPkgWithOptions(%s) error = %v", test.goarch, err)
			}
			defer tr.Module.Dispose()
			if len(tr.Functions) != 1 || len(tr.Untranslated) != 0 {
				t.Fatalf("Functions = %v, Untranslated = %v, want only example.com/fd.F", tr.Functions, tr.Untranslated)
			}
			if ir := tr.Module.String(); test.wantCall && !strings.Contains(ir, `@"example.com/fd.add"`) {
				t.Errorf("IR of %s does not call example.com/fd.add:\n%s", test.goarch, ir)
			}
		})
	}
}
