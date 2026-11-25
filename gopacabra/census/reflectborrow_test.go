package census

import (
	"testing"

	"golang.org/x/tools/go/ssa"
)

// Мініатюра оверлея llgo reflect (P12.5.7.2) у пакеті з шляхом runtime/abi:
// *Type тут — дескриптор типу рантайму. TypeOf дістає слово типу через
// unsafe.Pointer(&i), Type читає поле typ_, Set перезаписує x результатом
// assignTo. Контролі: тип, який ще не потрапив у мапу, ходить адресою,
// параметром і результатом — він тримає своє місце, а комірка в циклі
// бачить запис з попередньої ітерації.
const reflectBorrowSrc = `package p

import "unsafe"

type Type struct {
	Size uintptr
	Elem *Type
}

type eface struct {
	typ  *Type
	word unsafe.Pointer
}

type Value struct {
	typ_ *Type
	ptr  unsafe.Pointer
}

func TypeOf(i any) *Type {
	e := (*eface)(unsafe.Pointer(&i))
	return e.typ
}

func ValueOf(i any) Value {
	e := (*eface)(unsafe.Pointer(&i))
	return Value{e.typ, e.word}
}

func (v Value) typ() *Type { return v.typ_ }

func (v Value) Type() *Type { return v.typ() }

func hold(v Value)

func (v Value) assignTo(t *Type, target unsafe.Pointer) Value {
	if t == nil {
		hold(v)
	}
	return Value{t, target}
}

func (v Value) Set(x Value) {
	var target unsafe.Pointer
	if v.typ_ != nil {
		target = v.ptr
	}
	x = x.assignTo(v.typ(), target)
	*(*unsafe.Pointer)(v.ptr) = x.ptr
}

var cache = map[uintptr]*Type{}

func publish(t *Type) { cache[t.Size] = t }

func ident(t *Type) *Type { return t }

type structType struct {
	Type
	n int
}

func addr(st *structType) *Type { return &st.Type }

var sink *int

func loop(p *int, n int) {
	var c struct{ q *int }
	for i := 0; i < n; i++ {
		sink = c.q
		c.q = p
	}
}
`

// Мініатюра оверлейного reflect зі шляхом llgo: функції без тіла рантайм
// реалізує через linkname, census бере для них ручні зведення.
const reflectRuntimeSrc = `package p

import "unsafe"

type rtype struct{ size uintptr }

type Value struct {
	typ_ *rtype
	ptr  unsafe.Pointer
}

func typedmemmove(t *rtype, dst, src unsafe.Pointer)

func typedmemclr(t *rtype, ptr unsafe.Pointer)

func ifaceE2I(t *rtype, src any, dst unsafe.Pointer)

func (v Value) SetZero() { typedmemclr(v.typ_, v.ptr) }

func (v Value) Copy(x Value) { typedmemmove(v.typ_, v.ptr, x.ptr) }

func (v Value) SetIface(x any) { ifaceE2I(v.typ_, x, v.ptr) }
`

func TestReflectBorrow(t *testing.T) {
	type check struct {
		fn    string // функція пакета або метод Value
		param int
		want  string
		ok    func(Param) bool
	}
	borrowed := func(p Param) bool { return !p.Unknown && p.Side == 0 && p.Content == 0 && len(p.Alias) == 0 }
	escapes := func(p Param) bool { return p.Unknown || p.Side != 0 }
	aliased := func(p Param) bool { return !p.Unknown && len(p.Alias) > 0 }
	writes := func(p Param) bool { return borrowed(p) && p.Writes }
	tests := []struct {
		name, path, src string
		checks          []check
	}{
		{"abi", "github.com/xgo-dev/llgo/runtime/abi", reflectBorrowSrc, []check{
			{"TypeOf", 0, "позичання: слово типу не тримає коробку", borrowed},
			{"Type", 0, "позичання: typ_ — дескриптор", borrowed},
			{"Set", 0, "позичання із записом: v не йде в assignTo", writes},
			{"Set", 1, "втеча: x іде в assignTo", escapes},
			{"ValueOf", 0, "аліас: слово даних — коробка", aliased},
			{"publish", 0, "втеча: тип-параметр іде в мапу", escapes},
			{"ident", 0, "аліас: параметр-дескриптор", aliased},
			{"addr", 0, "аліас: адреса дескриптора", aliased},
			{"loop", 0, "втеча: запис попередньої ітерації", escapes},
		}},
		{"overlay", "github.com/xgo-dev/llgo/runtime/internal/lib/reflect", reflectRuntimeSrc, []check{
			{"SetZero", 0, "позичання із записом через v.ptr", writes},
			{"Copy", 0, "позичання із записом через v.ptr", writes},
			{"Copy", 1, "вміст x живе в *v.ptr", func(p Param) bool { return !p.Unknown && p.Side == 0 && p.Content != 0 }},
			{"SetIface", 0, "позичання із записом через v.ptr", writes},
			{"SetIface", 1, "втеча: x лягає в *v.ptr", func(p Param) bool { return !p.Unknown && p.Side != 0 }},
		}},
	}
	for _, tt := range tests {
		for _, mode := range []ssa.BuilderMode{ssa.SanityCheckFunctions, ssa.SanityCheckFunctions | ssa.GlobalDebug} {
			pkg := buildOnePackage(t, tt.path, tt.src, mode)
			funcs := ssaFuncs(pkg)
			value := pkg.Type("Value").Type()
			methods := pkg.Prog.MethodSets.MethodSet(value)
			for method := range methods.Methods() {
				funcs = append(funcs, pkg.Prog.MethodValue(method))
			}
			sums := packageSummaries(nil, funcs)
			for _, c := range tt.checks {
				fn := pkg.Func(c.fn)
				if fn == nil {
					fn = pkg.Prog.LookupMethod(value, pkg.Pkg, c.fn)
				}
				got := sums[fn].Params[c.param]
				if !c.ok(got) {
					t.Errorf("%s debug=%v: packageSummaries(%s).Params[%d] = %+v, want %s", tt.name, mode&ssa.GlobalDebug != 0, c.fn, c.param, got, c.want)
				}
			}
		}
	}
}
