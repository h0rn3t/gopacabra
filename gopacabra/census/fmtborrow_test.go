package census

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

// Форми друку fmt (P12.5.7.3). convertAssignRows у database/sql повідомляє
// про непідтримане призначення через fmt.Errorf("… %T …", src, dest): %T
// читає лише тип аргументу, методів не викликає. Базовий тип без методів
// друкується без виклику коду програми. Решта форм — контролі: %v і %w
// викликають методи значення невідомого типу, формат не константа, явний
// індекс аргументу, зайвий аргумент, тип з методом String, розгорнутий зріз.
const fmtBorrowSrc = `package p

import "fmt"

func typeOnly(src, dest any) error {
	return fmt.Errorf("storing driver.Value type %T into type %T", src, dest)
}

func basic(s string, n int) string {
	return fmt.Sprintf("%-8s|%*d|%%", s, 4, n)
}

func value(dest any) string { return fmt.Sprintf("%v", dest) }

func wrap(err error) error { return fmt.Errorf("scan: %w", err) }

func dynamic(format string, dest any) string { return fmt.Sprintf(format, dest) }

func indexed(dest any) string { return fmt.Sprintf("%[1]T", dest) }

func extra(a, b any) string { return fmt.Sprintf("%T", a, b) }

type name string

var held name

func (n name) String() string { held = n; return string(n) }

func stringer(n name) string { return fmt.Sprintf("%s", n) }

func spread(args ...any) string { return fmt.Sprintf("%T", args...) }
`

func TestFmtBorrow(t *testing.T) {
	borrowed := func(p Param) bool { return !p.Unknown && p.Side == 0 && p.Content == 0 && len(p.Alias) == 0 }
	unknown := func(p Param) bool { return p.Unknown }
	tests := []struct {
		fn    string
		param int
		want  string
		ok    func(Param) bool
	}{
		{"typeOnly", 0, "позичання: %T", borrowed},
		{"typeOnly", 1, "позичання: %T", borrowed},
		{"basic", 0, "позичання: рядок без методів", borrowed},
		{"value", 0, "невідомо: %v викликає методи", unknown},
		{"wrap", 0, "невідомо: %w", unknown},
		{"dynamic", 1, "невідомо: формат не константа", unknown},
		{"indexed", 0, "невідомо: явний індекс", unknown},
		{"extra", 0, "позичання: %T", borrowed},
		{"extra", 1, "невідомо: зайвий аргумент друкується як %v", unknown},
		{"stringer", 0, "невідомо: тип з методом String", unknown},
		{"spread", 0, "невідомо: розгорнутий зріз", unknown},
	}
	for _, mode := range []ssa.BuilderMode{ssa.SanityCheckFunctions, ssa.SanityCheckFunctions | ssa.GlobalDebug} {
		pkg := buildOnePackage(t, "example.com/p", fmtBorrowSrc, mode)
		sums := packageSummaries(nil, ssaFuncs(pkg))
		for _, tt := range tests {
			got := sums[pkg.Func(tt.fn)].Params[tt.param]
			if !tt.ok(got) {
				t.Errorf("debug=%v: packageSummaries(%s).Params[%d] = %+v, want %s", mode&ssa.GlobalDebug != 0, tt.fn, tt.param, got, tt.want)
			}
		}
	}
}
