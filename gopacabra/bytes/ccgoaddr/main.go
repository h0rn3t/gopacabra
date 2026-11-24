//go:build gopacabra

// Програма прийняття фази 12: форми адреси локальної змінної з коду, який
// ccgo переклав з C (modernc.org/sqlite). У gc такі змінні лежать на стеку
// (P12.1); тут вони не додають live pinned. Адреса, що втікає в глобальну
// змінну, лишається pinned.
package main

import (
	"os"
	"unsafe"
)

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

const (
	catPinned = 4
	n         = 64
)

var keep, keepArg unsafe.Pointer

// token — TToken з sqlite: {z, n}.
type token struct {
	z uintptr
	n uint32
}

// finish — параметр лише розіменовують, як pAll у _sqlite3FinishTrigger.
func finish(p unsafe.Pointer) uint64 {
	return uint64(*(*uintptr)(p)) + uint64(*(*uint32)(unsafe.Add(p, 8)))
}

// getMeta пише за адресою, як _sqlite3BtreeGetMeta.
func getMeta(p unsafe.Pointer) {
	*(*int32)(p) = 7
}

// beginTrans передає параметр далі, як _sqlite3BtreeBeginTrans.
func beginTrans(p unsafe.Pointer) {
	if p != nil {
		getMeta(p)
	}
}

// fill рекурсивно пише елементи за зсувом.
func fill(p unsafe.Pointer, n int) {
	if n > 0 {
		*(*int32)(unsafe.Add(p, 4*(n-1))) = int32(n)
		fill(p, n-1)
	}
}

// store зберігає параметр у глобальну змінну: адреса тікає.
func store(p unsafe.Pointer) {
	keepArg = p
}

// storeVia передає параметр у store.
func storeVia(p unsafe.Pointer) {
	store(p)
}

func argFinish(i int) uint64 {
	var tok token
	tok.z, tok.n = uintptr(i), 1
	return finish(unsafe.Pointer(&tok))
}

func argMeta() int32 {
	var meta int32
	beginTrans(unsafe.Pointer(&meta))
	return meta
}

func argFill() int32 {
	var regs [4]int32
	fill(unsafe.Pointer(&regs), len(regs))
	return regs[3]
}

func argEscapes() {
	var v int64 = 9
	storeVia(unsafe.Pointer(&v))
}

func add(a, b int32) int32 { return a + b }

// funcWord — слово значення функції, як __ccgo_fp у modernc.org/libc (P11.5.5).
func funcWord(f func(int32, int32) int32) uintptr {
	return *(*uintptr)(unsafe.Pointer(&struct{ f func(int32, int32) int32 }{f}))
}

// wordCall — виклик через слово, як FxMutexEnter у sqlite.go (P11.5.3).
func wordCall(fp uintptr) int32 {
	return (*(*func(int32, int32) int32)(unsafe.Pointer(&struct{ uintptr }{fp})))(2, 3)
}

// wordEscapes — адреса коробки йде в глобальну змінну: місце закріплене.
func wordEscapes(fp uintptr) {
	keep = unsafe.Pointer(&struct{ uintptr }{fp})
}

// expect повертає код виходу: 0, якщо зміна live pinned збіглася.
func expect(name string, run func(), pinned uint64) int {
	before := gopacabraLive(catPinned)
	run()
	if got := gopacabraLive(catPinned) - before; got != pinned {
		println(name, "live pinned delta", got, "want", pinned)
		return 4
	}
	return 0
}

func main() {
	fp := funcWord(add)
	var sum int32
	var total uint64
	cases := []struct {
		name   string
		run    func()
		pinned uint64
	}{
		{"wordcall", func() {
			for range n {
				sum += wordCall(fp)
			}
		}, 0},
		{"wordescapes", func() { wordEscapes(fp) }, 8},
		{"arg", func() {
			for i := range n {
				total += argFinish(i) + uint64(argMeta()) + uint64(argFill())
			}
		}, 0},
		{"argescapes", argEscapes, 8},
	}
	for _, c := range cases {
		if code := expect(c.name, c.run, c.pinned); code != 0 {
			os.Exit(code)
		}
	}
	if sum != 5*n || *(*uintptr)(keep) != fp || total != n*(n-1)/2+n*(1+7+4) || *(*int64)(keepArg) != 9 {
		println("values", sum, total)
		os.Exit(6)
	}
	os.Exit(0)
}
