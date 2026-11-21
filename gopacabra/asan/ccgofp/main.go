//go:build gopacabra

// Програма прийняття P11.5.4: статична функція в інтерфейсі — сталий глобал
// {fn, nil}, один на функцію, як funcval у gc. __ccgo_fp з modernc.org/libc
// лишає від інтерфейсу лише слово даних як uintptr у пам'яті поза купою, а
// sqlite порівнює ці слова (__ccgo_fp(Xsqlite3_free) == ...): коробка в купі
// давала б різні слова для однієї функції. Виклик іде після нових алокацій.
// Під -asan — код 0 без тексту ASan.
package main

import (
	"os"
	"unsafe"

	"github.com/xgo-dev/llgo/gopacabra/asan/ccgofp/fp"
)

// ccgoFP повторює __ccgo_fp з modernc.org/libc.
func ccgoFP(f interface{}) uintptr {
	type iface [2]uintptr
	return (*iface)(unsafe.Pointer(&f))[1]
}

func add(a, b int32) int32 { return a + b }

func sub(a, b int32) int32 { return a - b }

// table — пам'ять поза купою, як структури libc з указівниками на функції.
var table [64]uintptr

func call(fp uintptr, a, b int32) int32 {
	return (*(*func(int32, int32) int32)(unsafe.Pointer(&struct{ uintptr }{fp})))(a, b)
}

func main() {
	for i := range table {
		if i%2 == 0 {
			table[i] = ccgoFP(add)
		} else {
			table[i] = ccgoFP(sub)
		}
	}
	// Нові алокації займають пам'ять, звільнену після ccgoFP.
	var bufs [][]byte
	for i := range 256 {
		bufs = append(bufs, make([]byte, 16+i%16))
	}
	sum := int32(0)
	for i, fp := range table {
		sum += call(fp, int32(i), 1)
	}
	if want := int32(63 * 64 / 2); sum != want || len(bufs) != 256 {
		println("sum", sum, "want", want)
		os.Exit(2)
	}
	if table[0] != table[2] || table[0] == table[1] || ccgoFP(add) != table[0] {
		println("same function, different data words")
		os.Exit(3)
	}
	if ccgoFP(fp.Mul) != fp.MulFP() || call(fp.MulFP(), 6, 7) != 42 {
		println("data word of fp.Mul differs across packages")
		os.Exit(4)
	}
	println("asan-ccgofp-ok", sum)
}
