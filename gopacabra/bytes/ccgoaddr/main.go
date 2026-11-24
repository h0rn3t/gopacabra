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

var keep unsafe.Pointer

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
	}
	for _, c := range cases {
		if code := expect(c.name, c.run, c.pinned); code != 0 {
			os.Exit(code)
		}
	}
	if sum != 5*n || *(*uintptr)(keep) != fp {
		println("values", sum)
		os.Exit(6)
	}
	os.Exit(0)
}
