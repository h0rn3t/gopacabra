//go:build gopacabra

// Програма прийняття P8.6: field повертає свіжий внутрішній результат &x.f
// (P8.5), і викликач звільняє 16 байтів S через runtime.Base.
package main

import (
	"os"
	_ "unsafe"
)

//go:linkname gopacabraFreed github.com/xgo-dev/llgo/runtime/internal/runtime.Freed
func gopacabraFreed() uint64

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

// S — 16 байтів.
type S struct{ a, f int }

func field(n int) *int {
	x := &S{a: n}
	x.f = n * 2
	return &x.f
}

func use(n int) int {
	p := field(n)
	return *p + 1
}

func main() {
	beforeFreed := gopacabraFreed()
	beforeLive := gopacabraLive(1) // CatRCLocal
	if use(3) != 7 {
		os.Exit(2)
	}
	if got, want := gopacabraFreed()-beforeFreed, uint64(16); got != want {
		println("freed delta", got, "want", want)
		os.Exit(3)
	}
	if got, want := gopacabraLive(1), beforeLive; got != want {
		println("live rc-local", got, "want", want)
		os.Exit(4)
	}
	os.Exit(0)
}
