package census

import (
	"testing"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// encoding/json/internal/jsonwire.AppendQuote під gopacabra (P12.5.4):
// append(dst, src[i:n]...) копіює байти src у буфер результату, а виклик,
// що повертає свій аргумент (appendEscapedASCII), повертає й вміст буфера.
// Census вважав, що результат тримає src, тож []byte(s) у makeStringArshaler
// ішов разом з результатом у xe.Buf і лишався живим (rc-local call).
// Елементи-вказівники результат тримає й надалі: як Alias чи Holds.
const appendCopySrc = `package p

func esc(dst []byte) []byte { return append(dst, '\\') }

func Quote(dst, src []byte) []byte {
	dst = append(dst, src...)
	return esc(dst)
}

func QuoteString(dst []byte, s string) []byte {
	dst = append(dst, s...)
	return esc(dst)
}

func keep(dst []*int) []*int { return append(dst, nil) }

func Ptrs(dst, src []*int) []*int {
	dst = append(dst, src...)
	return keep(dst)
}
`

func TestAppendCopiesElements(t *testing.T) {
	pkg := buildOnePackage(t, "example.com/p", appendCopySrc, ssa.SanityCheckFunctions)
	var funcs []*ssa.Function
	for fn := range ssautil.AllFunctions(pkg.Prog) {
		if fn.Pkg == pkg {
			funcs = append(funcs, fn)
		}
	}
	sums := packageSummaries(nil, funcs)
	tests := []struct {
		fn   string
		held bool
	}{
		{fn: "Quote"},
		{fn: "QuoteString"},
		{fn: "Ptrs", held: true},
	}
	for _, tt := range tests {
		sum := sums[pkg.Func(tt.fn)]
		if sum == nil || len(sum.Params) != 2 {
			t.Errorf("packageSummaries(%s) = %+v, want 2 params", tt.fn, sum)
			continue
		}
		src := sum.Params[1]
		if held := len(src.Alias)+len(src.Holds) > 0; held != tt.held {
			t.Errorf("packageSummaries(%s).Params[1] = Alias %v Holds %v, want result holds src = %v", tt.fn, src.Alias, src.Holds, tt.held)
		}
	}
}
