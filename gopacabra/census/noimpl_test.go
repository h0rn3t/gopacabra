package census

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

// Форма database/sql (*Rows).scanLocked: dest іде в метод необов'язкового
// інтерфейсу драйвера. Якщо жоден тип програми не оголошує ScanColumn,
// приведення r.(ColumnScanner) не вдається ніколи, і виклик нічого не
// утримує (P12.5.7.4).
const noImplSrc = `package p

type Rows interface{ Next() bool }

type ColumnScanner interface {
	ScanColumn(i int, dest any) error
}

func scan(r Rows, dest any) error {
	if s, ok := r.(ColumnScanner); ok {
		return s.ScanColumn(0, dest)
	}
	return nil
}
`

// Контроль: тип програми оголошує ScanColumn і тримає dest.
const withImplSrc = noImplSrc + `
var held any

type impl struct{}

func (impl) Next() bool { return false }

func (*impl) ScanColumn(i int, dest any) error { held = dest; return nil }
`

func TestNoImplementerBorrows(t *testing.T) {
	t.Cleanup(func() { PublishProgram(nil, nil) })
	scanDest := func(pkg *ssa.Package) Param {
		t.Helper()
		fn := pkg.Func("scan")
		sum := packageSummaries(nil, []*ssa.Function{fn})[fn]
		if sum == nil || len(sum.Params) != 2 {
			t.Fatalf("packageSummaries(scan) = %+v, want 2 Params", sum)
		}
		return sum.Params[1]
	}
	for _, mode := range []ssa.BuilderMode{ssa.SanityCheckFunctions, ssa.SanityCheckFunctions | ssa.GlobalDebug} {
		noImpl := buildOnePackage(t, "example.com/p", noImplSrc, mode)
		withImpl := buildOnePackage(t, "example.com/p", withImplSrc, mode)

		// Без зведень програми множина типів невідома.
		PublishProgram(nil, nil)
		if p := scanDest(noImpl); !p.Unknown {
			t.Errorf("mode %v, без програми: scan dest = %+v, want Unknown", mode, p)
		}
		// Інша програма не говорить нічого про цю.
		PublishProgram(withImpl.Prog, nil)
		if p := scanDest(noImpl); !p.Unknown {
			t.Errorf("mode %v, чужа програма: scan dest = %+v, want Unknown", mode, p)
		}
		if p := scanDest(withImpl); !p.Unknown && p.Side == 0 {
			t.Errorf("mode %v, є реалізація: scan dest = %+v, want Unknown чи втеча", mode, p)
		}
		PublishProgram(noImpl.Prog, nil)
		if p := scanDest(noImpl); p.Unknown || p.Side != 0 || p.Content != 0 || len(p.Alias) != 0 || len(p.Holds) != 0 {
			t.Errorf("mode %v, без реалізацій: scan dest = %+v, want позичання", mode, p)
		}
	}
}
