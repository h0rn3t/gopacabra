package census

import (
	"testing"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// internal/exportdata.lookupGorootExport (P11.5.0.3): замикання, що захопило
// sync.Once, іде в sync.Map.LoadOrStore, а LoadOrStore лише зберігає параметр
// у пам'яті приймача і повертає його. Замикання отримує Inc у виклику, а комірку, яку воно
// тримає, Dec звільняв на виході функції — наступний виклик замикання з мапи
// читав звільнену пам'ять. Те саме для поля структури, яку callee зберіг, і
// для callee без зведення (виклик через інтерфейс).
const keptContentSrc = `package p

type M struct{ m map[string]any }

func (m *M) LoadOrStore(k string, v any) (any, bool) {
	if a, ok := m.m[k]; ok {
		return a, true
	}
	m.m[k] = v
	return v, false
}

func (m *M) Read(v any) bool { return v != nil }

type T struct{ s *string }

func closure(m *M, k string) string {
	var once string
	f, _ := m.LoadOrStore(k, func() string { return once })
	return f.(func() string)()
}

func field(m *M, k string) string {
	s := k
	f, _ := m.LoadOrStore(k, &T{s: &s})
	return *f.(*T).s
}

type keeper interface{ LoadOrStore(k string, v any) (any, bool) }

func unknown(c keeper, k string) string {
	s := k
	f, _ := c.LoadOrStore(k, &T{s: &s})
	return *f.(*T).s
}

func read(m *M, k string) bool {
	var once string
	return m.Read(func() string { return once })
}
`

func TestKeptArgumentContentsLive(t *testing.T) {
	pkg := buildOnePackage(t, "example.com/p", keptContentSrc, ssa.SanityCheckFunctions)
	var funcs []*ssa.Function
	for fn := range ssautil.AllFunctions(pkg.Prog) {
		if fn.Pkg == pkg {
			funcs = append(funcs, fn)
		}
	}
	place := PlaceAll(funcs)
	cell := map[string]ssa.Instruction{}
	for _, fn := range funcs {
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				if alloc, ok := instr.(*ssa.Alloc); ok && (alloc.Comment == "once" || alloc.Comment == "s") {
					cell[fn.Name()] = alloc
				}
			}
		}
	}
	tests := []struct {
		fn  string
		dec bool
	}{
		{fn: "closure"},
		{fn: "field"},
		{fn: "unknown"},
		// Read не зберігає параметр: комірку можна звільнити.
		{fn: "read", dec: true},
	}
	for _, tt := range tests {
		site := cell[tt.fn]
		if site == nil {
			t.Errorf("%s: no Alloc of the cell", tt.fn)
			continue
		}
		if _, got := place.DecUses[site]; got != tt.dec {
			t.Errorf("PlaceAll(%s).DecUses[cell] present = %v, want %v (category %q)", tt.fn, got, tt.dec, place.Cats[site])
		}
	}
}
