//go:build gopacabra

// Програма прийняття P12.5.1: комірка змінної, яку захоплює defer-замикання,
// звільняється після запуску відкладених викликів. Форма — з коду ccgo
// (modernc.org/libc, sqlite): `if __ccgo_strace { defer func() { trc(r) }() }`.
// До виправлення кожен виклик лишав у live rc-local 8 байтів комірки.
package main

import (
	"os"
	_ "unsafe"
)

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

const (
	catRCLocal = 1
	n          = 64
	strace     = false
)

var (
	verbose bool
	traced  int
)

func trc(r int) { traced += r }

// constDefer — форма ccgo: умова — константа false.
func constDefer(i int) (r int) {
	if strace {
		defer func() { trc(r) }()
	}
	return i + 1
}

// varDefer — умова відома лише під час виконання.
func varDefer(i int) (r int) {
	if verbose {
		defer func() { trc(r) }()
	}
	return i + 1
}

// alwaysDefer — відкладений виклик змінює результат після return.
func alwaysDefer(i int) (r int) {
	defer func() { r++ }()
	return i + 1
}

// localDefer — захоплена локальна змінна, не результат.
func localDefer(i int) int {
	r := i + 1
	if verbose {
		defer func() { trc(r) }()
	}
	r++
	return r
}

// loopDefer — defer у циклі: кожен виклик читає комірку r після return.
func loopDefer(i int) (r int) {
	for j := range 3 {
		defer func() { r += j }()
	}
	return i
}

// recovered — відкладений виклик пише r на шляху panic після recover.
func recovered(i int) (r int) {
	defer func() {
		if recover() != nil {
			r = 2 * i
		}
	}()
	if i >= 0 {
		panic("recovered")
	}
	return i
}

// expect повертає код виходу: 0, якщо сума результатів і зміна live rc-local
// збіглися з очікуваними.
func expect(name string, f func(int) int, sum int, live uint64) int {
	before := gopacabraLive(catRCLocal)
	got := 0
	for i := range n {
		got += f(i)
	}
	if got != sum {
		println(name, "sum", got, "want", sum)
		return 2
	}
	if delta := gopacabraLive(catRCLocal) - before; delta != live {
		println(name, "live rc-local delta", delta, "want", live)
		return 4
	}
	return 0
}

func main() {
	base := n * (n - 1) / 2
	cases := []struct {
		name string
		f    func(int) int
		sum  int
		live uint64
	}{
		{"constdefer", constDefer, base + n, 0},
		{"vardefer", varDefer, base + n, 0},
		{"alwaysdefer", alwaysDefer, base + 2*n, 0},
		{"localdefer", localDefer, base + 2*n, 0},
	}
	for _, c := range cases {
		if code := expect(c.name, c.f, c.sum, c.live); code != 0 {
			os.Exit(code)
		}
	}
	verbose = true
	if code := expect("vardefer-on", varDefer, base+n, 0); code != 0 {
		os.Exit(code)
	}
	if traced != base+n {
		println("traced", traced, "want", base+n)
		os.Exit(5)
	}
	// Тут перевіряються лише значення: комірка в циклі й на шляху panic
	// може лишитись живою, але не звільненою зарано.
	for i := range n {
		if got := loopDefer(i); got != i+3 {
			println("loopdefer", i, got)
			os.Exit(6)
		}
		if got := recovered(i); got != 2*i {
			println("recovered", i, got)
			os.Exit(7)
		}
	}
	os.Exit(0)
}
