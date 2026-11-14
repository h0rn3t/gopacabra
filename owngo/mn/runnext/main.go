//go:build owngo && owngomn

// Програма прийняття P-NEW-09.13: G, яку розбудила G, що не віддає P, не
// чекає на витіснення, коли інший P вільний. Будильник шле в канал і
// крутиться, доки одержувач не відповість через атомарну змінну; розбуджений
// одержувач лягає в runnext P будильника. Вільний P мусить його вкрасти.
// Без крадіжки runnext кожне коло триває до витіснення (~10 мс). Медіана
// затримки — рядок mn-runnext-us; код 0 лише якщо вона менша за 1 мс.
package main

/*
#include <unistd.h>
*/
import "C"

import (
	"fmt"
	"os"
	"runtime"
	"slices"
	"sync/atomic"
	"time"
)

const rounds = 100

func main() {
	runtime.GOMAXPROCS(2)
	C.alarm(10)
	ch := make(chan int)
	var got atomic.Int64
	go func() {
		for v := range ch {
			got.Store(int64(v))
		}
	}()
	lat := make([]time.Duration, rounds)
	for i := range rounds {
		time.Sleep(200 * time.Microsecond) // одержувач встигає запаркуватися
		start := time.Now()
		ch <- i + 1
		for got.Load() != int64(i+1) {
		}
		lat[i] = time.Since(start)
	}
	close(ch)
	slices.Sort(lat)
	median := lat[rounds/2]
	fmt.Printf("mn-runnext-us %d\n", median.Microseconds())
	if median >= time.Millisecond {
		println("runnext: woken goroutine waited", median.String())
		os.Exit(1)
	}
}
