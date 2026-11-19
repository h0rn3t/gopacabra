//go:build gopacabra

// Програма прийняття P11.0: швидкі шляхи мапи з рядковим ключем і ключем-
// вказівником хешують ключ без купової копії, тож N операцій не додають
// байтів до gopacabra live pinned.
package main

import (
	"os"
	"strconv"
	_ "unsafe" // go:linkname
)

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

const (
	catPinned = 4
	n         = 64
)

func main() {
	// Понад вісім ключів: мапа має кілька кошиків, тож і читання хешує ключ.
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "key-" + strconv.Itoa(i)
	}
	ptrs := make([]*int, n)
	for i := range ptrs {
		ptrs[i] = new(int)
	}
	m := make(map[string]int, n)
	pm := make(map[*int]int, n)

	before := gopacabraLive(catPinned)
	sum := 0
	for i, k := range keys {
		m[k] = i        // mapassign_faststr
		pm[ptrs[i]] = i // mapassign_fast64ptr / fast32ptr
	}
	for _, k := range keys {
		sum += m[k] // mapaccess1_faststr
		if v, ok := m[k]; ok {
			sum += v // mapaccess2_faststr
		}
	}
	for _, k := range keys {
		delete(m, k) // mapdelete_faststr
	}
	if got := gopacabraLive(catPinned) - before; got != 0 {
		println("live pinned delta", got, "want 0")
		os.Exit(4)
	}
	if want := (n - 1) * n; sum != want || len(m) != 0 || len(pm) != n {
		println("values", sum, want, len(m), len(pm))
		os.Exit(6)
	}
	os.Exit(0)
}
