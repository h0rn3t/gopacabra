package cl

import (
	"regexp"
	"strings"
	"testing"

	"github.com/xgo-dev/llvm"
)

// gopacabraOpCall — виклик операції рантайму, яку codegen ставить місцю.
var gopacabraOpCall = regexp.MustCompile(`call [^\n]*@"[^"]*runtime\.(AllocZ|AllocU|AllocCounted|NoteAlloc|FreeAccounted|DecAccounted|DecCyclic|Inc|RegionCreate|RegionAlloc|FreeRegion)"\(`)

// TestGopacabraStatSites — P-NEW-02.1: під GopacabraStat кожна операція рантайму
// місця static, rc-local і cyclic отримує свій лічильник, а таблиця пакета
// називає позицію, категорію, причину й операцію. Без GopacabraStat ні
// лічильників, ні таблиці немає.
func TestGopacabraStatSites(t *testing.T) {
	const src = `package foo

type Tree struct {
	Left, Right *Tree
}

func NewTree(depth int) *Tree {
	if depth > 0 {
		return &Tree{Left: NewTree(depth - 1), Right: NewTree(depth - 1)}
	}
	return &Tree{}
}

func local(depth int) int {
	a := NewTree(depth)
	if a.Left == nil {
		return 1
	}
	return 2
}

func staticNew() int {
	p := new(int)
	*p = 1
	return *p
}

func closureUse() int {
	p := new(int)
	*p = 1
	f := func() int { return *p }
	return f() + *p
}

func kept(depth int) int {
	a := NewTree(depth)
	a.Left = nil
	if a.Right == nil {
		return 1
	}
	return 2
}
`
	mod := compileGopacabra(t, src, Options{GopacabraFree: true, GopacabraStat: true})
	if err := llvm.VerifyModule(mod, llvm.ReturnStatusAction); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"foo.NewTree", "foo.NewTree$region", "foo.local", "foo.staticNew", "foo.closureUse", "foo.kept"} {
		body := mustNamedFunction(t, mod, name).String()
		ops := len(gopacabraOpCall.FindAllString(body, -1))
		counters := strings.Count(body, `atomicrmw add ptr @"foo.__gopacabra_site$`)
		if ops == 0 || counters != ops {
			t.Errorf("%s: %d site counters for %d runtime ops, want one per op", name, counters, ops)
		}
	}
	ir := mod.String()
	for _, row := range []string{
		`:23 static - alloc`,
		`:23 static - note_alloc`,
		`:23 static - free_accounted`,
		`:29 rc-local [^ "]+ alloc_counted`,
		`:29 rc-local [^ "]+ dec_accounted`,
		`:9 cyclic [^ "]+ alloc_counted`,
		// local лише читає дерево — корінь регіону (P-NEW-05.4); kept пише
		// в нього й лишається на DecCyclic.
		`:9 cyclic [^ "]+ alloc_region`,
		`:15 cyclic - region_create`,
		`:15 cyclic - free_region`,
		`:36 cyclic - dec_cyclic`,
	} {
		if !regexp.MustCompile(row + `"`).MatchString(ir) {
			t.Errorf("site table has no row matching %q", row)
		}
	}
	// Конструктор модуля, а не init: пакет LLGoPackage = "link" init не має.
	if !strings.Contains(mustNamedFunction(t, mod, "foo.__gopacabra_sites$ctor").String(), "runtime.RegisterSites") {
		t.Error("foo.__gopacabra_sites$ctor has no runtime.RegisterSites call")
	}
	if !regexp.MustCompile(`__gopacabra_sites\$ctor.*section "\.init_array"|@llvm\.global_ctors = appending .*__gopacabra_sites\$ctor`).MatchString(ir) {
		t.Error("module does not run foo.__gopacabra_sites$ctor as a constructor")
	}
	// Запис .init_array ніхто не використовує: без llvm.compiler.used його
	// викидає GlobalDCE під -lto, і таблиця мовчки зникає.
	if strings.Contains(ir, `@"foo.__gopacabra_sites$init" =`) &&
		!regexp.MustCompile(`@llvm\.compiler\.used = appending global .*@"foo\.__gopacabra_sites\$init"`).MatchString(ir) {
		t.Error("foo.__gopacabra_sites$init is not in llvm.compiler.used, want it kept under LTO")
	}

	off := compileGopacabra(t, src, Options{GopacabraFree: true}).String()
	for _, s := range []string{"__gopacabra_site", "RegisterSites", "atomicrmw"} {
		if strings.Contains(off, s) {
			t.Errorf("GopacabraStat false: module has %q, want no site counters", s)
		}
	}
}

func compileGopacabra(t *testing.T, src string, opts Options) llvm.Module {
	t.Helper()
	ssaPkg, _, files := buildGoSSAPkg(t, src)
	prog := newLLSSAProg(t)
	t.Cleanup(prog.Dispose)
	pkg, _, err := NewPackageExWithEmbedMetaOptions(prog, nil, nil, nil, ssaPkg, files, nil, false, opts)
	if err != nil {
		t.Fatal(err)
	}
	return pkg.Module()
}

// TestGopacabraStatOwnedAppend — P-NEW-03.6: під GopacabraStat власницький
// append кличе OwnedAppendCapStat, який рахує копіювання за достатньої
// ємності. Без GopacabraStat виклик той самий, що до задачі: OwnedAppendCap.
func TestGopacabraStatOwnedAppend(t *testing.T) {
	const src = `package foo

func staticAppend(s, extra []int) int {
	s = append(s, extra...)
	return len(s)
}
`
	for _, tc := range []struct {
		stat       bool
		want, none string
	}{
		{stat: true, want: `runtime.OwnedAppendCapStat"(`, none: `runtime.OwnedAppendCap"(`},
		{stat: false, want: `runtime.OwnedAppendCap"(`, none: `runtime.OwnedAppendCapStat"(`},
	} {
		mod := compileGopacabra(t, src, Options{GopacabraFree: true, GopacabraStat: tc.stat})
		body := mustNamedFunction(t, mod, "foo.staticAppend").String()
		if !strings.Contains(body, tc.want) || strings.Contains(body, tc.none) {
			t.Errorf("GopacabraStat %v: foo.staticAppend has no call %s or has %s:\n%s", tc.stat, tc.want, tc.none, body)
		}
	}
}
