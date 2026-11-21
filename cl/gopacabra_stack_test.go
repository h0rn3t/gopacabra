package cl

import (
	"regexp"
	"strings"
	"testing"

	"github.com/xgo-dev/llvm"
)

// TestGopacabraStackStatic — P-NEW-04.2: місце static сталого розміру до 64 КиБ,
// чий об'єкт не переживає ітерацію циклу, стає alloca без AllocZ і
// FreeAccounted. Об'єкт, який phi, контейнер чи defer несе в наступну
// ітерацію, і об'єкт більше 64 КиБ лишаються в купі.
func TestGopacabraStackStatic(t *testing.T) {
	const src = `package foo

func use(p *int) int { return *p }

func fill(t *[8]int, i int) { t[i&7] = i }

func staticNew() int {
	p := new(int)
	*p = 1
	return *p
}

func inLoop(n int) int {
	acc := 0
	for i := 0; i < n; i++ {
		var flag [8]int
		fill(&flag, i)
		acc += flag[i&7]
	}
	return acc
}

func appendArgs(s []*int, n int) []*int {
	for i := 0; i < n; i++ {
		s = append(s, nil)
	}
	return s
}

func carried(n int) int {
	var prev *int
	acc := 0
	for i := 0; i < n; i++ {
		p := new(int)
		*p = i
		if prev != nil {
			acc += *prev
		}
		prev = p
	}
	return acc
}

func stored(n int) int {
	var arr [4]*int
	for i := 0; i < 4; i++ {
		p := new(int)
		*p = i
		arr[i] = p
	}
	return *arr[0] + *arr[3]
}

func deferred(n int) {
	for i := 0; i < n; i++ {
		p := new(int)
		*p = i
		defer use(p)
	}
}

func big() int {
	p := new([10000]int)
	p[5] = 1
	return p[5]
}
`
	mod := compileGopacabraStack(t, src, true)
	if err := llvm.VerifyModule(mod, llvm.ReturnStatusAction); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		heap bool
	}{
		{"foo.staticNew", false},
		{"foo.inLoop", false},
		{"foo.appendArgs", false},
		{"foo.carried", true},
		{"foo.stored", true},
		{"foo.deferred", true},
		{"foo.big", true},
	}
	for _, tt := range tests {
		ir := mustNamedFunction(t, mod, tt.name).String()
		if got := strings.Contains(ir, "runtime.AllocZ"); got != tt.heap {
			t.Errorf("%s has AllocZ = %v, want %v", tt.name, got, tt.heap)
		}
		if !tt.heap && strings.Contains(ir, "runtime.FreeAccounted") {
			t.Errorf("%s has FreeAccounted, want none for a stack object", tt.name)
		}
		// Купове місце мусить бути саме static, інакше кейс нічого не перевіряє.
		if tt.heap && !regexp.MustCompile(`NoteAlloc"\(i64 \d+, i32 0\)`).MatchString(ir) {
			t.Errorf("%s has no static NoteAlloc:\n%s", tt.name, ir)
		}
	}
	if ir := mustNamedFunction(t, mod, "foo.staticNew").String(); !strings.Contains(ir, "alloca i64") {
		t.Errorf("foo.staticNew has no alloca i64:\n%s", ir)
	}

	off := compileGopacabraStack(t, src, false)
	if ir := mustNamedFunction(t, off, "foo.staticNew").String(); !strings.Contains(ir, "runtime.AllocZ") || !strings.Contains(ir, "runtime.FreeAccounted") {
		t.Errorf("GopacabraStack false: foo.staticNew lost AllocZ or FreeAccounted:\n%s", ir)
	}
}

func compileGopacabraStack(t *testing.T, src string, stack bool) llvm.Module {
	t.Helper()
	ssaPkg, _, files := buildGoSSAPkg(t, src)
	prog := newLLSSAProg(t)
	t.Cleanup(prog.Dispose)
	pkg, _, err := NewPackageExWithEmbedMetaOptions(
		prog, nil, nil, nil, ssaPkg, files, nil, false,
		Options{GopacabraFree: true, GopacabraStack: stack},
	)
	if err != nil {
		t.Fatal(err)
	}
	return pkg.Module()
}

// TestGopacabraConstIface — P-NEW-04.4: стала невказівникового типу в інтерфейсі
// місця static бере слово даних з незмінної глобальної змінної, без AllocU і
// FreeAccounted. Змінна в інтерфейсі і стала, що тікає через return,
// лишаються в купі.
func TestGopacabraConstIface(t *testing.T) {
	const src = `package foo

type Errno uintptr

func (e Errno) Error() string { return "errno" }

const EINTR = Errno(4)

func isEINTR(err error) bool { return err == EINTR }

func isBoom(v any) bool { return v == "boom" }

func boxVar(x int, v any) bool { return v == any(x) }

func retConst() error { return EINTR }
`
	mod := compileGopacabraStack(t, src, true)
	if err := llvm.VerifyModule(mod, llvm.ReturnStatusAction); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		heap bool
	}{
		{"foo.isEINTR", false},
		{"foo.isBoom", false},
		{"foo.boxVar", true},
		{"foo.retConst", true},
	}
	for _, tt := range tests {
		ir := mustNamedFunction(t, mod, tt.name).String()
		if got := strings.Contains(ir, "runtime.AllocU"); got != tt.heap {
			t.Errorf("%s has AllocU = %v, want %v:\n%s", tt.name, got, tt.heap, ir)
		}
		if !tt.heap && (strings.Contains(ir, "runtime.FreeAccounted") || strings.Contains(ir, "runtime.NoteAlloc")) {
			t.Errorf("%s has FreeAccounted or NoteAlloc, want none for a constant:\n%s", tt.name, ir)
		}
	}
	// Слово даних EINTR — адреса приватної незмінної змінної зі значенням 4.
	var data []string
	for g := mod.FirstGlobal(); !g.IsNil(); g = llvm.NextGlobal(g) {
		init := g.Initializer()
		if g.Linkage() == llvm.PrivateLinkage && g.IsGlobalConstant() && !init.IsNil() &&
			init.Type() == mod.Context().Int64Type() && init.ZExtValue() == 4 {
			data = append(data, g.Name())
		}
	}
	if len(data) == 0 {
		t.Errorf("no private constant i64 4 global for EINTR:\n%s", mod.String())
	}

	off := compileGopacabraStack(t, src, false)
	if ir := mustNamedFunction(t, off, "foo.isEINTR").String(); !strings.Contains(ir, "runtime.AllocU") {
		t.Errorf("GopacabraStack false: foo.isEINTR lost AllocU:\n%s", ir)
	}
}

// TestGopacabraFuncIface — P11.5.4: статична функція в інтерфейсі бере слово
// даних з незмінної глобальної змінної {fn, nil}, однієї на функцію, як
// funcval у gc: без алокації в будь-якій категорії census, і однакові слова
// даних для однієї функції (__ccgo_fp з modernc.org/libc їх порівнює).
// Замикання з прив'язками лишається в купі.
func TestGopacabraFuncIface(t *testing.T) {
	const src = `package foo

import "unsafe"

func add(a, b int32) int32 { return a + b }

func ccgoFP(f interface{}) uintptr {
	type iface [2]uintptr
	return (*iface)(unsafe.Pointer(&f))[1]
}

var sink any

func local() uintptr { return ccgoFP(add) }

func escape() any { return add }

func global() { sink = add }

func static() bool { var x any = add; return x != nil }

func closure(k int32) any { return func(a int32) int32 { return a * k } }
`
	mod := compileGopacabraStack(t, src, true)
	if err := llvm.VerifyModule(mod, llvm.ReturnStatusAction); err != nil {
		t.Fatal(err)
	}
	add := mustNamedFunction(t, mod, "foo.add")
	var funcvals []llvm.Value
	for g := mod.FirstGlobal(); !g.IsNil(); g = llvm.NextGlobal(g) {
		init := g.Initializer()
		if g.IsGlobalConstant() && !init.IsNil() && init.Type().TypeKind() == llvm.StructTypeKind &&
			init.OperandsCount() == 2 && init.Operand(0) == add && init.Operand(1).IsNull() {
			funcvals = append(funcvals, g)
		}
	}
	if len(funcvals) != 1 {
		t.Fatalf("got %d constant {foo.add, null} globals, want 1:\n%s", len(funcvals), mod.String())
	}
	ref := "@" + funcvals[0].Name()
	if strings.ContainsAny(funcvals[0].Name(), "$.") {
		ref = `@"` + funcvals[0].Name() + `"`
	}
	for _, name := range []string{"foo.local", "foo.escape", "foo.global", "foo.static"} {
		ir := mustNamedFunction(t, mod, name).String()
		if strings.Contains(ir, "runtime.Alloc") || strings.Contains(ir, "runtime.NoteAlloc") {
			t.Errorf("%s allocates, want the data word of foo.add from %s:\n%s", name, ref, ir)
		}
		if !strings.Contains(ir, ref) {
			t.Errorf("%s does not reference %s:\n%s", name, ref, ir)
		}
	}
	if ir := mustNamedFunction(t, mod, "foo.closure").String(); !strings.Contains(ir, "runtime.AllocU") {
		t.Errorf("foo.closure lost AllocU of the closure box:\n%s", ir)
	}
}
