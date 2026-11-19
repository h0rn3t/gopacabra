//go:build gopacabra && gopacabramn

// Програма прийняття P-NEW-09.12: Ready, що спізнився, не будить наступне
// паркування. Кожна з пар G паркується rounds разів на тому самому
// очікувачі, як G у netpoll на pollDesc.rw: перед паркуванням кладе його
// адресу в slot, а G-будильник забирає адресу CAS-ом і кличе Ready. G, що
// повернулася з Park, а її адреса досі в slot, прокинулася без Ready —
// у netpoll це "corrupted polldesc". Код 0 лише без жодного такого
// пробудження; час — рядок mn-lateready-ms.
package main

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

//go:linkname waiterReset github.com/xgo-dev/llgo/runtime/internal/runtime.MNWaiterResetForTesting
func waiterReset(w unsafe.Pointer)

//go:linkname waiterPark github.com/xgo-dev/llgo/runtime/internal/runtime.MNWaiterParkForTesting
func waiterPark(w unsafe.Pointer)

//go:linkname waiterReady github.com/xgo-dev/llgo/runtime/internal/runtime.MNWaiterReadyForTesting
func waiterReady(w unsafe.Pointer)

const pairs, rounds = 4, 200_000

func main() {
	runtime.GOMAXPROCS(2 * pairs)
	start := time.Now()
	var spurious atomic.Int64
	var wg sync.WaitGroup
	for range pairs {
		// Очікувач: *g і notified з запасом.
		w := unsafe.Pointer(new([4]uintptr))
		var slot atomic.Pointer[[4]uintptr]
		var stop atomic.Bool
		wg.Go(func() {
			for range rounds {
				waiterReset(w)
				slot.Store((*[4]uintptr)(w))
				waiterPark(w)
				if slot.CompareAndSwap((*[4]uintptr)(w), nil) {
					spurious.Add(1)
				}
			}
			stop.Store(true)
		})
		wg.Go(func() {
			for !stop.Load() {
				if slot.CompareAndSwap((*[4]uintptr)(w), nil) {
					waiterReady(w)
					runtime.Gosched()
				}
			}
		})
	}
	wg.Wait()
	fmt.Printf("mn-lateready-ms %d\n", time.Since(start).Milliseconds())
	if n := spurious.Load(); n > 0 {
		fmt.Printf("%d wakeups without Ready\n", n)
		os.Exit(1)
	}
}
