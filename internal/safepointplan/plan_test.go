package safepointplan

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func TestBackedges(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "straight line", src: `package p; func f(n int) int { return n + 1 }`},
		{name: "loop", src: `package p; func f(n int) { for n > 0 { n-- } }`, want: 1},
		{name: "nested loops", src: `package p; func f(n int) { for i := 0; i < n; i++ { for j := 0; j < n; j++ {} } }`, want: 2},
		{
			name: "irreducible loop",
			src: `package p
func f(n int) {
	if n > 0 { goto left }
right:
	n--
	if n > 0 { goto left }
	return
left:
	n--
	if n > 0 { goto right }
}`,
			want: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fn := buildFunction(t, test.src)
			polls := Backedges(fn)
			if len(polls) != test.want {
				t.Fatalf("Backedges returned %d polls, want %d", len(polls), test.want)
			}
			for instr := range polls {
				switch instr.(type) {
				case *ssa.If, *ssa.Jump:
				default:
					t.Errorf("poll instruction is %T, want a block terminator", instr)
				}
			}
		})
	}
}

// TestUnboundedBackedges: під M:N (P-NEW-09.10.1) гніздо циклів зі сталими
// початком, кроком і межею, де зворотні ребра разом проходяться не більше
// maxBoundedTrip разів, опитування не має: воно скінченне, і G поступиться у
// зовнішньому циклі чи на вході виклику.
func TestUnboundedBackedges(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "range constant", src: `package p; func f(a *[8]int) { for k := range 8 { a[k]++ } }`},
		{name: "counted up inclusive", src: `package p; func f(a *[8]int) { for i := 1; i <= 49; i++ { a[0]++ } }`},
		{name: "counted down", src: `package p; func f(a *[8]int) { for i := 7; i >= 0; i-- { a[i]++ } }`},
		{name: "step two", src: `package p; func f(a *[8]int) { for i := 0; i != 8; i += 2 { a[i]++ } }`},
		{name: "at limit", src: `package p; func f(a *[8]int) { for i := 0; i < 4096; i++ { a[0]++ } }`},
		{name: "over limit", src: `package p; func f(a *[8]int) { for i := 0; i < 4097; i++ { a[0]++ } }`, want: 1},
		{name: "variable bound", src: `package p; func f(n int) { for i := 0; i < n; i++ {} }`, want: 1},
		{name: "condition loop", src: `package p; func f(n int) { for n > 0 { n-- } }`, want: 1},
		{name: "nested bounded", src: `package p; func f(a *[8]int) { for i := 0; i < 49; i++ { for k := range 8 { a[k] += i }; for k := range 8 { a[k]-- } } }`},
		{name: "nested over limit", src: `package p; func f(a *[8]int) { for i := 0; i < 1000; i++ { for k := range 8 { a[k] += i } } }`, want: 1},
		{name: "inner unbounded", src: `package p; func f(n int) { for i := 0; i < 8; i++ { for n > 0 { n-- } } }`, want: 2},
		{name: "int8 wraps", src: `package p; func f(a *[8]int) { for i := int8(0); i <= 127; i++ { a[0]++ } }`, want: 1},
		{name: "unsigned wraps", src: `package p; func f(a *[8]int) { for i := uint(5); i >= 0; i-- { a[0]++ } }`, want: 1},
		{name: "counter reset", src: `package p; func f(a *[8]int) { for i := 0; i < 8; i++ { if a[0] > 0 { i = 0 } } }`, want: 1},
		{name: "no step", src: `package p; func f(a *[8]int) { for i := 0; i < 8; { a[0]++ } }`, want: 1},
		{name: "wrong direction", src: `package p; func f(a *[8]int) { for i := 0; i < 8; i-- { a[0]++ } }`, want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fn := buildFunction(t, test.src)
			if got := len(UnboundedBackedges(fn)); got != test.want {
				t.Fatalf("UnboundedBackedges returned %d polls, want %d (Backedges: %d)", got, test.want, len(Backedges(fn)))
			}
		})
	}
}

// TestEntryPollsNeeds: опитування на вході потрібне лише функції, що може
// лежати на циклі графа викликів (P-NEW-09.10.1, P-NEW-07.9): вона кличе
// невідому ціль — значення функції, інтерфейс, функцію без тіла, go чи
// defer — або статичну функцію, якій опитування потрібне. Виклик листової
// функції, зокрема ланцюжок таких, опитування не додає.
func TestEntryPollsNeeds(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{name: "leaf", src: `package p; func f(p *int) int { return *p + 1 }`},
		{name: "leaf loop", src: `package p; func f(n int) { for n > 0 { n-- } }`},
		{name: "builtin", src: `package p; func f(s []int) []int { return append(s, len(s)) }`},
		{name: "static leaf call", src: `package p; func g(x int) int { return x + 1 }; func f(x int) int { return g(x) }`},
		{name: "leaf chain", src: `package p; func h() int { return 1 }; func g() int { return h() + 1 }; func f() int { return g() + g() }`},
		{name: "callee loops", src: `package p; func g(n int) { for n > 0 { n-- } }; func f(n int) { g(n) }`},
		{name: "concrete method", src: `package p; type T int; func (t T) m() int { return int(t) }; func f(t T) int { return t.m() }`},
		{name: "leaf closure", src: `package p; func f(x int) int { g := func() int { return x }; return g() }`},
		{name: "recursion", src: `package p; func f(n int) int { if n == 0 { return 0 }; return f(n-1) + 1 }`, want: true},
		{name: "mutual recursion", src: `package p; func g(n int) int { return f(n) + 1 }; func f(n int) int { if n == 0 { return 0 }; return g(n - 1) }`, want: true},
		{name: "callee recursion", src: `package p; func g(n int) int { if n == 0 { return 0 }; return g(n - 1) }; func f(n int) int { return g(n) }`, want: true},
		{name: "callee indirect", src: `package p; func g(h func()) { h() }; func f(h func()) { g(h) }`, want: true},
		{name: "indirect call", src: `package p; func f(g func()) { g() }`, want: true},
		{name: "interface call", src: `package p; type I interface{ M() }; func f(i I) { i.M() }`, want: true},
		{name: "go statement", src: `package p; func f(g func()) { go g() }`, want: true},
		{name: "go leaf", src: `package p; func g() {}; func f() { go g() }`, want: true},
		{name: "defer", src: `package p; func f(g func()) { defer g() }`, want: true},
		{name: "no body", src: `package p; import "strings"; func f(s string) string { return strings.ToUpper(s) }`, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := new(EntryPolls).Needs(buildFunction(t, test.src)); got != test.want {
				t.Fatalf("Needs(f) = %v, want %v\n%s", got, test.want, test.src)
			}
		})
	}
}

// TestEntryPollsNeedsOtherPackage: тіло функції іншого пакета враховується,
// як bits.ReverseBytes64 у fannkuch-redux (P-NEW-07.9).
func TestEntryPollsNeedsOtherPackage(t *testing.T) {
	const dep = `package q
func Leaf(x uint64) uint64 { return x<<8 | x>>56 }
func Calls(g func()) { g() }
`
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{name: "leaf", src: `package p; import "q"; func f(x uint64) uint64 { return q.Leaf(x) }`},
		{name: "indirect inside", src: `package p; import "q"; func f(g func()) { q.Calls(g) }`, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := new(EntryPolls).Needs(buildWithDep(t, dep, test.src)); got != test.want {
				t.Fatalf("Needs(f) = %v, want %v\n%s", got, test.want, test.src)
			}
		})
	}
}

// TestEntryPollsMemo: результат кешується, і порядок запитів його не змінює.
func TestEntryPollsMemo(t *testing.T) {
	fn := buildFunction(t, `package p
func h() int { return 1 }
func g(n int) int { if n == 0 { return h() }; return f(n - 1) }
func f(n int) int { return g(n) }
`)
	pkg := fn.Pkg
	var polls EntryPolls
	for _, name := range []string{"h", "g", "f", "h", "f", "g"} {
		want := name != "h"
		if got := polls.Needs(pkg.Func(name)); got != want {
			t.Errorf("Needs(%s) = %v, want %v", name, got, want)
		}
	}
}

func TestBackedgesNil(t *testing.T) {
	if got := Backedges(nil); got != nil {
		t.Fatalf("Backedges(nil) = %v, want nil", got)
	}
}

func TestBackedgesDeepGraphDoesNotUseCallStack(t *testing.T) {
	const blockCount = 100_000
	blocks := make([]*ssa.BasicBlock, blockCount)
	for i := range blocks {
		blocks[i] = &ssa.BasicBlock{
			Index:  i,
			Instrs: []ssa.Instruction{new(ssa.Jump)},
		}
		if i != 0 {
			blocks[i-1].Succs = []*ssa.BasicBlock{blocks[i]}
		}
	}
	blocks[blockCount-1].Succs = []*ssa.BasicBlock{blocks[0]}

	if got := len(Backedges(&ssa.Function{Blocks: blocks})); got != 1 {
		t.Fatalf("Backedges returned %d polls, want 1", got)
	}
}

func buildFunction(t *testing.T, src string) *ssa.Function {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _, err := ssautil.BuildPackage(
		&types.Config{Importer: importer.Default()},
		fset,
		types.NewPackage("p", "p"),
		[]*ast.File{file},
		ssa.InstantiateGenerics,
	)
	if err != nil {
		t.Fatal(err)
	}
	return pkg.Func("f")
}

// buildWithDep будує пакет q з depSrc з тілами функцій і пакет p з src, що
// його імпортує, в одній програмі SSA, як internal/build.
func buildWithDep(t *testing.T, depSrc, src string) *ssa.Function {
	t.Helper()
	fset := token.NewFileSet()
	prog := ssa.NewProgram(fset, ssa.InstantiateGenerics)
	check := func(path, src string, imp types.Importer) *ssa.Package {
		file, err := parser.ParseFile(fset, path+".go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		info := &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Implicits:  make(map[ast.Node]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
			Scopes:     make(map[ast.Node]*types.Scope),
			Instances:  make(map[*ast.Ident]types.Instance),
		}
		typ, err := (&types.Config{Importer: imp}).Check(path, fset, []*ast.File{file}, info)
		if err != nil {
			t.Fatal(err)
		}
		pkg := prog.CreatePackage(typ, []*ast.File{file}, info, true)
		pkg.Build()
		return pkg
	}
	dep := check("q", depSrc, nil)
	imp := importerFunc(func(path string) (*types.Package, error) { return dep.Pkg, nil })
	return check("p", src, imp).Func("f")
}

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }
