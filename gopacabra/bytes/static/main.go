//go:build gopacabra

// Програма прийняття P1.5: одне купове місце static звільняє свої байти.
// Масив більший за межу стека 64 КиБ (P-NEW-04.2), тож лишається в купі.
package main

import (
	"os"
	_ "unsafe"
)

//go:linkname gopacabraFreed github.com/xgo-dev/llgo/runtime/internal/runtime.Freed
func gopacabraFreed() uint64

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

const size = 64<<10 + 8

func staticSite() byte {
	p := new([size]byte)
	p[0] = 1
	return p[0]
}

func main() {
	beforeFreed := gopacabraFreed()
	beforeLive := gopacabraLive(0) // CatStatic
	if staticSite() != 1 {
		os.Exit(2)
	}
	if got, want := gopacabraFreed()-beforeFreed, uint64(size); got != want {
		println("freed delta", got, "want", want)
		os.Exit(3)
	}
	if got, want := gopacabraLive(0), beforeLive; got != want {
		println("live static", got, "want", want)
		os.Exit(4)
	}
	os.Exit(0)
}
