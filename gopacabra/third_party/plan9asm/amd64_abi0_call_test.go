//go:build !llgo
// +build !llgo

package plan9asm

import (
	"regexp"
	"strings"
	"testing"

	"github.com/xgo-dev/llvm"
)

// CALL без <ABIInternal> — це ABI0: аргументи й результати лежать у кадрі
// викликача за 0(SP). Після оптимізації кадр має згорнутися, і викликана
// функція отримує саме параметри викликача.
func TestTranslateAMD64ABI0CallUsesFrame(t *testing.T) {
	pkg := mustGoPackage(t, "test/pkg", `package testpkg
func Add(a, b int64) int64
func Sub(a, b int64) int64
func Len(s string) int64
func Internal(a, b int64) int64
func add(a, b int64) int64
func divmod(a, b int64) (q, r int64)
func strlen(s string) int64
`)
	tests := []struct {
		name string
		asm  string
		want string // регулярний вираз для тіла оптимізованої функції
	}{
		{
			name: "scalar",
			asm: `TEXT ·Add(SB),$24-24
	MOVQ a+0(FP), AX
	MOVQ AX, 0(SP)
	MOVQ b+8(FP), AX
	MOVQ AX, 8(SP)
	CALL ·add(SB)
	MOVQ 16(SP), AX
	MOVQ AX, ret+16(FP)
	RET
`,
			want: `(?s)%(\w+) = (?:tail )?call i64 @"test/pkg\.add"\(i64 %arg0, i64 %arg1\).*ret i64 %(\w+)`,
		},
		{
			name: "two results",
			asm: `TEXT ·Sub(SB),$32-24
	MOVQ a+0(FP), AX
	MOVQ AX, 0(SP)
	MOVQ b+8(FP), AX
	MOVQ AX, 8(SP)
	CALL ·divmod(SB)
	MOVQ 16(SP), AX
	SUBQ 24(SP), AX
	MOVQ AX, ret+16(FP)
	RET
`,
			want: `(?s)call \{ i64, i64 \} @"test/pkg\.divmod"\(i64 %arg0, i64 %arg1\).*extractvalue \{ i64, i64 \} %\w+, 0.*extractvalue \{ i64, i64 \} %\w+, 1.*sub i64`,
		},
		{
			name: "string arg",
			asm: `TEXT ·Len(SB),$24-24
	MOVQ s_base+0(FP), AX
	MOVQ AX, 0(SP)
	MOVQ s_len+8(FP), AX
	MOVQ AX, 8(SP)
	CALL ·strlen(SB)
	MOVQ 16(SP), AX
	MOVQ AX, ret+16(FP)
	RET
`,
			want: `call i64 @"test/pkg\.strlen"\(\{ ptr, i64 \} %arg0\)`,
		},
		{
			// <ABIInternal> лишає регістрову передачу аргументів.
			name: "abiinternal",
			asm: `TEXT ·Internal(SB),$8-24
	MOVQ a+0(FP), AX
	MOVQ b+8(FP), BX
	MOVQ $0, 0(SP)
	CALL ·add<ABIInternal>(SB)
	MOVQ AX, ret+16(FP)
	RET
`,
			want: `(?s)%(\w+) = (?:tail )?call i64 @"test/pkg\.add"\(i64 %arg0, i64 %arg1\).*ret i64 %(\w+)`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tr, err := TranslateGoModule(pkg, []byte(test.asm), GoModuleOptions{
				GOOS:         "linux",
				GOARCH:       "amd64",
				TargetTriple: "x86_64-unknown-linux-gnu",
				ResolveSym:   testResolveSym("test/pkg"),
			})
			if err != nil {
				t.Fatalf("TranslateGoModule(%s) error = %v", test.name, err)
			}
			defer tr.Module.Dispose()
			pbo := llvm.NewPassBuilderOptions()
			defer pbo.Dispose()
			if err := tr.Module.RunPasses("default<O2>", llvm.TargetMachine{}, pbo); err != nil {
				t.Fatalf("RunPasses(default<O2>) error = %v", err)
			}
			body := definedFunc(tr.Module.String(), tr.Functions[0].ResolvedSymbol)
			if !regexp.MustCompile(test.want).MatchString(body) {
				t.Errorf("optimized %s does not match %q:\n%s", tr.Functions[0].ResolvedSymbol, test.want, body)
			}
		})
	}
}

// definedFunc повертає текст визначення функції sym з IR модуля.
func definedFunc(ir, sym string) string {
	start := strings.Index(ir, `define `)
	for start >= 0 {
		end := strings.Index(ir[start:], "\n}\n")
		if end < 0 {
			return ""
		}
		fn := ir[start : start+end+2]
		if strings.Contains(strings.SplitN(fn, "\n", 2)[0], `@"`+sym+`"`) {
			return fn
		}
		next := strings.Index(ir[start+end:], "\ndefine ")
		if next < 0 {
			return ""
		}
		start += end + next + 1
	}
	return ""
}
