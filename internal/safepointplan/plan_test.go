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

// TestNeedsEntryPoll: функції без викликів Go (P-NEW-09.10.1) опитування на
// вході не потрібне — її цикли скінченні або опитуються самі; рекурсія і
// будь-який виклик опитування лишають.
func TestNeedsEntryPoll(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{name: "leaf", src: `package p; func f(p *int) int { return *p + 1 }`},
		{name: "leaf loop", src: `package p; func f(n int) { for n > 0 { n-- } }`},
		{name: "builtin", src: `package p; func f(s []int) []int { return append(s, len(s)) }`},
		{name: "recursion", src: `package p; func f(n int) int { if n == 0 { return 0 }; return f(n-1) + 1 }`, want: true},
		{name: "indirect call", src: `package p; func f(g func()) { g() }`, want: true},
		{name: "go statement", src: `package p; func f(g func()) { go g() }`, want: true},
		{name: "defer", src: `package p; func f(g func()) { defer g() }`, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NeedsEntryPoll(buildFunction(t, test.src)); got != test.want {
				t.Fatalf("NeedsEntryPoll = %v, want %v", got, test.want)
			}
		})
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
