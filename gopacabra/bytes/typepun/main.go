//go:build gopacabra

// Програма прийняття P11.0.1: змінна, чию адресу лише перетворюють в інший
// вказівник і читають чи пишуть у межах функції, живе на стеку, як у gc.
// Адреса, що втікає в глобальну змінну, лишається pinned.
package main

import (
	"math"
	"os"
	"strconv"
	"unsafe"
)

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

const (
	catPinned = 4
	n         = 64
)

var keep unsafe.Pointer

// pun — type-pun у коді модуля: читання і запис через інший тип.
func pun(f float64) uint64 {
	*(*uint64)(unsafe.Pointer(&f)) ^= 1 << 63
	return *(*uint64)(unsafe.Pointer(&f))
}

// ccgoFP — __ccgo_fp з modernc.org/libc: читання елемента масиву через
// інший тип (P11.5.4).
func ccgoFP(f interface{}) uintptr {
	type iface [2]uintptr
	return (*iface)(unsafe.Pointer(&f))[1]
}

// ifaceType — читання поля структури через інший тип.
func ifaceType(f interface{}) uintptr {
	return (*struct{ typ, data uintptr })(unsafe.Pointer(&f)).typ
}

// escapes — адреса параметра йде в глобальну змінну: місце закріплене.
func escapes(f float64) {
	keep = unsafe.Pointer(&f)
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
	var bits uint64
	var text string
	cases := []struct {
		name   string
		run    func()
		pinned uint64
	}{
		{"math", func() {
			for i := range n {
				f := float64(i) + 0.5
				bits += math.Float64bits(math.Float64frombits(math.Float64bits(f)))
				bits += uint64(math.Float32bits(math.Float32frombits(math.Float32bits(float32(f)))))
			}
		}, 0},
		{"strconv", func() {
			for i := range n {
				text = strconv.FormatFloat(float64(i)/3, 'g', -1, 64)
			}
		}, 0},
		{"pun", func() {
			for i := range n {
				bits += pun(float64(i))
			}
		}, 0},
		{"ccgofp", func() {
			for range n {
				bits += uint64(ccgoFP(math.Abs) & 1)
				bits += uint64(ifaceType(math.Abs) & 1)
			}
		}, 0},
		{"escapes", func() { escapes(1.5) }, 8},
	}
	for _, c := range cases {
		if code := expect(c.name, c.run, c.pinned); code != 0 {
			os.Exit(code)
		}
	}
	if bits == 0 || text != "21" || *(*float64)(keep) != 1.5 {
		println("values", bits, text)
		os.Exit(6)
	}
	os.Exit(0)
}
