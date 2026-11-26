package census

import (
	"slices"
	"testing"

	"golang.org/x/tools/go/ssa"
)

// v, ok := x.(T), m[k] чи <-ch повертають значення з параметра так само, як
// форма без ok. До виправлення Extract кортежу, що не є викликом, губив
// аліас: fiber toFiberHandler(arg) позичав обробник, і App.Add звільняв
// замикання, яке тримає маршрутизатор (P12.5.7.3).
const commaOkSrc = `package p

type H func() error

type T struct{ x int }

func assertFunc(arg any) (H, bool) { h, ok := arg.(H); return h, ok }
func assertPtr(arg any) *T          { p, _ := arg.(*T); return p }
func assertIface(arg any) error     { e, _ := arg.(error); return e }
func lookup(m map[string]*T) *T     { v, _ := m["k"]; return v }
func recv(ch chan *T) *T            { v, _ := <-ch; return v }
`

func TestCommaOkKeepsAlias(t *testing.T) {
	pkg := buildOnePackage(t, "example.com/p", commaOkSrc, ssa.SanityCheckFunctions)
	sums := packageSummaries(nil, ssaFuncs(pkg))
	for _, name := range []string{"assertFunc", "assertPtr", "assertIface", "lookup", "recv"} {
		sum := sums[pkg.Func(name)]
		if sum == nil || len(sum.Params) != 1 || !slices.Equal(sum.Params[0].Alias, []int{0}) {
			t.Errorf("packageSummaries(%s) = %+v, want Params[0].Alias [0]", name, sum)
		}
	}
}
