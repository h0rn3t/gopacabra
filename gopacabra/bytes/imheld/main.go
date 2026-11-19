//go:build gopacabra

// Програма прийняття P5.2: невідомий виклик утримує конкатенацію.
// v1 позначає її rc-local: call і Dec не ставить, тож 8 байтів лишаються живими.
package main

import (
	"os"
	_ "unsafe"
)

//go:linkname gopacabraFreed github.com/xgo-dev/llgo/runtime/internal/runtime.Freed
func gopacabraFreed() uint64

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

func touch(s string) { _ = len(s) }

func heldSite(a, b string, f func(string)) byte {
	s := a + b
	f(s)
	return s[0]
}

func main() {
	beforeFreed := gopacabraFreed()
	beforeStatic := gopacabraLive(0) // CatStatic
	beforeLocal := gopacabraLive(1)  // CatRCLocal
	if heldSite("abcd", "efgh", touch) != 'a' {
		os.Exit(2)
	}
	if got, want := gopacabraFreed(), beforeFreed; got != want {
		println("freed", got, "want", want)
		os.Exit(3)
	}
	if got, want := gopacabraLive(0), beforeStatic; got != want {
		println("live static", got, "want", want)
		os.Exit(4)
	}
	if got, want := gopacabraLive(1)-beforeLocal, uint64(8); got != want {
		println("live rc-local delta", got, "want", want)
		os.Exit(5)
	}
	os.Exit(0)
}
