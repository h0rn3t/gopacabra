//go:build gopacabra

// Програма прийняття P6.3: невідомий виклик тримає кільце двох вузлів.
// v1 позначає вузли cyclic: shared і DecCyclic не ставить, тож їхні байти
// лишаються серед живих cyclic.
package main

import (
	"os"
	"unsafe"
)

//go:linkname gopacabraFreed github.com/xgo-dev/llgo/runtime/internal/runtime.Freed
func gopacabraFreed() uint64

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

type node struct {
	next *node
	val  int
}

func touch(n *node) { _ = n.val }

func heldSite(f func(*node)) int {
	a := &node{val: 1}
	b := &node{val: 2}
	a.next = b
	b.next = a
	f(a)
	return a.val + b.next.val
}

func main() {
	beforeFreed := gopacabraFreed()
	beforeLive := gopacabraLive(3) // CatCyclic
	if heldSite(touch) != 2 {
		os.Exit(2)
	}
	if got := gopacabraFreed(); got != beforeFreed {
		println("freed", got, "want", beforeFreed)
		os.Exit(3)
	}
	want := uint64(2 * unsafe.Sizeof(node{}))
	if got := gopacabraLive(3) - beforeLive; got != want {
		println("live cyclic delta", got, "want", want)
		os.Exit(4)
	}
	os.Exit(0)
}
