//go:build gopacabra && gopacabramn

// Програма прийняття P-NEW-09.10: на GOMAXPROCS=1 горутина крутиться в
// порожньому циклі очікування на атомарній змінній і не віддає P сама. Інша
// горутина мусить завершитися менш ніж за 1 с: sysmon просить витіснення, а
// safepoint на зворотному ребрі циклу віддає P. Без витіснення програма
// висить, і alarm завершує її з ненульовим кодом. Час — рядок mn-preempt-ms.
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
	"time"
)

func main() {
	runtime.GOMAXPROCS(1)
	C.alarm(5)
	var stop atomic.Bool
	go func() {
		for !stop.Load() {
		}
	}()
	runtime.Gosched() // P переходить до циклу
	start := time.Now()
	done := make(chan struct{})
	go func() { close(done) }()
	<-done
	elapsed := time.Since(start)
	stop.Store(true)
	fmt.Printf("mn-preempt-ms %.1f\n", float64(elapsed.Microseconds())/1000)
	if elapsed > time.Second {
		println("preempt: second goroutine waited", elapsed.String())
		os.Exit(2)
	}
}
