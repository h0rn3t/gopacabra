package plan9asm

import (
	"maps"
	"slices"
	"testing"
)

func TestTranslateKeepsUntranslatableFuncsOut(t *testing.T) {
	pkg := mustTestPackage(t, "example.com/u", "package u\nfunc F()\nfunc G()\nfunc H()\n")
	tests := []struct {
		name      string
		asm       string
		wantFuncs []string
		wantBad   []string
	}{
		{
			name:      "plain",
			asm:       "TEXT ·F(SB),NOSPLIT,$0-0\n\tRET\nTEXT ·G(SB),NOSPLIT,$0-0\n\tINVALID\n\tRET\nTEXT ·H(SB),NOSPLIT,$0-0\n\tRET\n",
			wantFuncs: []string{"example.com/u.F", "example.com/u.H"},
			wantBad:   []string{"example.com/u.G"},
		},
		{
			// Вимкнена гілка не перекладається, але й не робить F неперекладною.
			name:      "conditional",
			asm:       "#ifdef GOAMD64_v3\nTEXT ·F(SB),NOSPLIT,$0-0\n\tRET\n#else\nTEXT ·F(SB),NOSPLIT,$0-0\n\tINVALID\n\tRET\n#endif\nTEXT ·G(SB),NOSPLIT,$0-0\n\tINVALID\n\tRET\n",
			wantFuncs: []string{"example.com/u.F"},
			wantBad:   []string{"example.com/u.G"},
		},
		{
			name:    "all untranslatable",
			asm:     "TEXT ·G(SB),NOSPLIT,$0-0\n\tINVALID\n\tRET\nTEXT ·H(SB),NOSPLIT,$0-0\n\tINVALID\n\tRET\n",
			wantBad: []string{"example.com/u.G", "example.com/u.H"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tr, err := TranslateSourceModuleForPkgWithOptions(pkg, "u_amd64.s", []byte(test.asm), "linux", "amd64", TranslateOptions{GOAMD64: "v3"})
			if err != nil {
				t.Fatalf("TranslateSourceModuleForPkgWithOptions(%s) error = %v", test.name, err)
			}
			defer tr.Module.Dispose()
			var funcs []string
			for _, fn := range tr.Functions {
				funcs = append(funcs, fn.ResolvedSymbol)
			}
			if !slices.Equal(funcs, test.wantFuncs) {
				t.Errorf("Functions = %v, want %v", funcs, test.wantFuncs)
			}
			if bad := slices.Sorted(maps.Keys(tr.Untranslated)); !slices.Equal(bad, test.wantBad) {
				t.Errorf("Untranslated = %v, want %v", tr.Untranslated, test.wantBad)
			}
		})
	}
}
