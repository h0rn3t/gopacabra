//go:build gopacabra

// Програма прийняття P3.5: невідомий виклик утримує базове місце rc-local,
// тож його 8 байтів лишаються серед живих.
package main

import (
	"os"
	_ "unsafe"
)

//go:linkname gopacabraFreed github.com/xgo-dev/llgo/runtime/internal/runtime.Freed
func gopacabraFreed() uint64

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

func touch(p *[8]byte) { _ = p[0] }

func held(f func(*[8]byte)) byte {
	p := new([8]byte)
	p[0] = 7
	f(p)
	return p[0]
}

func main() {
	beforeFreed := gopacabraFreed()
	beforeLive := gopacabraLive(1) // CatRCLocal
	if held(touch) != 7 {
		os.Exit(2)
	}
	if got, want := gopacabraFreed(), beforeFreed; got != want {
		println("freed", got, "want", want)
		os.Exit(3)
	}
	if got, want := gopacabraLive(1)-beforeLive, uint64(8); got != want {
		println("live rc-local delta", got, "want", want)
		os.Exit(4)
	}
	os.Exit(0)
}
