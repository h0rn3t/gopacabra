//go:build gopacabra

// Програма прийняття P5.2: доведена конкатенація static звільняє свої 8 байтів.
package main

import (
	"os"
	_ "unsafe"
)

//go:linkname gopacabraFreed github.com/xgo-dev/llgo/runtime/internal/runtime.Freed
func gopacabraFreed() uint64

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

func concatSite(a, b string) byte {
	s := a + b
	return s[0]
}

func main() {
	beforeFreed := gopacabraFreed()
	beforeLive := gopacabraLive(0) // CatStatic
	if concatSite("abcd", "efgh") != 'a' {
		os.Exit(2)
	}
	if got, want := gopacabraFreed()-beforeFreed, uint64(8); got != want {
		println("freed delta", got, "want", want)
		os.Exit(3)
	}
	if got, want := gopacabraLive(0), beforeLive; got != want {
		println("live static", got, "want", want)
		os.Exit(4)
	}
	os.Exit(0)
}
