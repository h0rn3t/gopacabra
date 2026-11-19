//go:build gopacabra

// Програма прийняття P8.6: невідомий виклик утримує &s.f, тож 16 байтів S
// лишаються серед живих rc-local.
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

func touch(p *int) { *p++ }

func held(f func(*int), n int) int {
	s := S{a: n}
	get := func() int { return s.a }
	f(&s.f)
	return get() + s.f
}

func main() {
	beforeFreed := gopacabraFreed()
	beforeLive := gopacabraLive(1) // CatRCLocal
	if held(touch, 4) != 5 {
		os.Exit(2)
	}
	if got, want := gopacabraFreed(), beforeFreed; got != want {
		println("freed", got, "want", want)
		os.Exit(3)
	}
	if got, want := gopacabraLive(1)-beforeLive, uint64(16); got != want {
		println("live rc-local delta", got, "want", want)
		os.Exit(4)
	}
	os.Exit(0)
}
