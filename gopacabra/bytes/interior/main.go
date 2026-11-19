//go:build gopacabra

// Програма прийняття P8.6: локальний &s.f (P8.4) не забирає в місця Dec,
// і 16 байтів S ідуть у звільнені, коли лічильник доходить до нуля.
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

func interior(n int) int {
	s := S{a: n}
	get := func() int { return s.a }
	p := &s.f
	for i := 0; i < n; i++ {
		*p += i
	}
	return get() + *p
}

func main() {
	beforeFreed := gopacabraFreed()
	beforeLive := gopacabraLive(1) // CatRCLocal
	if interior(4) != 10 {
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
