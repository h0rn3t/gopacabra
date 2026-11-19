//go:build gopacabra

// Програма прийняття P4.2: невідомий виклик утримує місце поза main.
// v1 позначає його rc-local: shared і не ставить Dec, тож 8 байтів лишаються живими.
package main

import (
	"os"
	_ "unsafe"

	"github.com/xgo-dev/llgo/gopacabra/bytes/outheld/site"
)

//go:linkname gopacabraFreed github.com/xgo-dev/llgo/runtime/internal/runtime.Freed
func gopacabraFreed() uint64

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

func touch(p *[8]byte) { _ = p[0] }

func main() {
	beforeFreed := gopacabraFreed()
	beforeStatic := gopacabraLive(0) // CatStatic
	beforeLocal := gopacabraLive(1)  // CatRCLocal
	if site.Held(touch) != 7 {
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
