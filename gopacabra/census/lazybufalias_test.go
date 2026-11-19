package census

import (
	"slices"
	"testing"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// path.Clean під gopacabra: параметр лягає в поле локальної структури
// (lazybuf.s), а результат — підрядок цього поля, прочитаний методом через
// вказівник на структуру. Результат тримає аргумент, тож рядок
// `string(buf)` у path.Join живе, доки живе результат. До виправлення
// census звільняв його одразу після виклику, і embed.FS.ReadFile читав
// звільнену пам'ять (P11.5.0.3).
const lazybufAliasSrc = `package p

type lazybuf struct {
	s   string
	buf []byte
	w   int
}

func (b *lazybuf) string() string {
	if b.buf == nil {
		return b.s[:b.w]
	}
	return string(b.buf[:b.w])
}

func Clean(path string) string {
	out := lazybuf{s: path}
	out.w = len(path)
	return out.string()
}
`

func TestLazybufParamAliasesResult(t *testing.T) {
	pkg := buildOnePackage(t, "example.com/p", lazybufAliasSrc, ssa.SanityCheckFunctions)
	var funcs []*ssa.Function
	for fn := range ssautil.AllFunctions(pkg.Prog) {
		if fn.Pkg == pkg {
			funcs = append(funcs, fn)
		}
	}
	sums := packageSummaries(nil, funcs)
	if sum := sums[pkg.Func("Clean")]; sum == nil || len(sum.Params) != 1 || !slices.Equal(sum.Params[0].Alias, []int{0}) {
		t.Errorf("packageSummaries(Clean) = %+v, want Params[0].Alias [0]", sum)
	}
}
