//go:build gopacabra

// Програма прийняття P4.2: базове купове місце static у пакеті поза main
// звільняє свої site.Size байтів.
package main

import (
	"os"
	_ "unsafe"

	"github.com/xgo-dev/llgo/gopacabra/bytes/outstatic/site"
)

//go:linkname gopacabraFreed github.com/xgo-dev/llgo/runtime/internal/runtime.Freed
func gopacabraFreed() uint64

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

func main() {
	beforeFreed := gopacabraFreed()
	beforeLive := gopacabraLive(0) // CatStatic
	if site.Static() != 1 {
		os.Exit(2)
	}
	if got, want := gopacabraFreed()-beforeFreed, uint64(site.Size); got != want {
		println("freed delta", got, "want", want)
		os.Exit(3)
	}
	if got, want := gopacabraLive(0), beforeLive; got != want {
		println("live static", got, "want", want)
		os.Exit(4)
	}
	os.Exit(0)
}
