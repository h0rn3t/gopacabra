//go:build owngo && owngomn

// Програма прийняття P-NEW-09.14: G, що повернулася з виклику C без P,
// алокує з власної купи потоку M. Коли M знову бере P, ця купа не має
// губитися: наступне виконання без P на тій самій M бере її ж, тож куп не
// більше, ніж P і M разом. На GOMAXPROCS=1 одна G спить у C.usleep, а друга
// крутиться, тож sysmon забирає P у виклику і віддає його другій G; перша
// G повертається без P і алокує. Код 0 лише якщо куп не більше за P + M.
package main

/*
#include <unistd.h>
*/
import "C"

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
)

//go:linkname heaps github.com/xgo-dev/llgo/runtime/internal/runtime.MNAllocHeapsForTesting
func heaps() int

//go:linkname ms github.com/xgo-dev/llgo/runtime/internal/runtime.MNCountForTesting
func ms() int

const rounds = 500

var sink []byte

func main() {
	runtime.GOMAXPROCS(1)
	var stop atomic.Bool
	go func() {
		for range rounds {
			C.usleep(500)
			sink = make([]byte, 64)
			runtime.Gosched()
		}
		stop.Store(true)
	}()
	for !stop.Load() {
		runtime.Gosched()
	}
	h, m := heaps(), ms()
	fmt.Printf("mn-plessheap heaps %d ms %d\n", h, m)
	if h > m+runtime.GOMAXPROCS(0) {
		fmt.Fprintf(os.Stderr, "mn-plessheap: %d allocator heaps for %d Ms and 1 P\n", h, m)
		os.Exit(1)
	}
}
