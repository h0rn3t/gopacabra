//go:build gopacabra && gopacabrastat

// Програма прийняття P-NEW-09.4: кадр defer не лишається в купі. Функція з
// defer, що виходить раніше за defer, не виділяє нічого; функція, що доходить
// до defer, виділяє щонайбільше вузол відкладеного виклику (його звільняє
// FreeDeferNode; під -O0 вузла немає). Код 0 лише якщо gopacabra ops alloc зріс
// не більше за межу, а gopacabra freed і сума gopacabra live не змінились. Паніка з recover проходить через той самий
// кадр і має повернути значення.
package main

import (
	"os"
	"strconv"
	"sync"
	_ "unsafe"
)

//go:linkname opCount github.com/xgo-dev/llgo/runtime/internal/runtime.OpCount
func opCount(op int32) uint64

//go:linkname gopacabraFreed github.com/xgo-dev/llgo/runtime/internal/runtime.Freed
func gopacabraFreed() uint64

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

const (
	opAlloc  = 0 // opAlloc у runtime/internal/runtime
	catCount = 7 // accountCats у runtime/internal/runtime
)

// guard повторює database/sql.closingMutex: швидкий шлях виходить до defer.
type guard struct {
	mu   sync.Mutex
	fast bool
	n    int
}

//go:noinline
func (g *guard) enter() {
	if g.fast {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
}

//go:noinline
func recovered(i int) (r int) {
	defer func() {
		if recover() != nil {
			r = i
		}
	}()
	panic("p")
}

func liveAll() uint64 {
	var sum uint64
	for cat := int32(0); cat < catCount; cat++ {
		sum += gopacabraLive(cat)
	}
	return sum
}

// check звіряє зміну лічильників після f з межею кількості виділень.
func check(name string, code int, maxAllocs uint64, f func()) {
	a, fr, live := opCount(opAlloc), gopacabraFreed(), liveAll()
	f()
	if got := opCount(opAlloc) - a; got > maxAllocs {
		println(name, "alloc delta", got, "want at most", maxAllocs)
		os.Exit(code)
	}
	if got := gopacabraFreed() - fr; got != 0 {
		println(name, "freed delta", got, "want 0")
		os.Exit(code + 1)
	}
	if got := liveAll() - live; got != 0 {
		println(name, "live delta", got, "want 0")
		os.Exit(code + 2)
	}
}

func main() {
	n := 1000
	if len(os.Args) > 1 {
		n, _ = strconv.Atoi(os.Args[1])
	}
	g := &guard{fast: true}
	check("fast", 2, 0, func() {
		for range n {
			g.enter()
		}
	})
	g.fast = false
	check("slow", 5, uint64(n), func() {
		for range n {
			g.enter()
		}
	})
	if g.n != n {
		println("count", g.n, "want", n)
		os.Exit(8)
	}
	for i := range 3 {
		if got := recovered(i); got != i {
			println("recovered", got, "want", i)
			os.Exit(9)
		}
	}
}
