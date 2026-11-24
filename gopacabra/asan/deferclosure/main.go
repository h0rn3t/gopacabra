//go:build gopacabra

// Фікстура P12.5.1: комірку, яку захоплює defer-замикання, читають і пишуть
// відкладені виклики після return, у циклі й на шляху panic. Під ASan процес
// має завершитись без звіту: комірка не звільняється раніше за RunDefers.
package main

import "os"

var verbose = true

func alwaysDefer(i int) (r int) {
	defer func() { r++ }()
	return i + 1
}

func condDefer(i int) (r int) {
	if verbose {
		defer func() { r *= 2 }()
	}
	return i
}

func loopDefer(i int) (r int) {
	for j := range 3 {
		defer func() { r += j }()
	}
	return i
}

func recovered(i int) (r int) {
	defer func() {
		if recover() != nil {
			r = 2 * i
		}
	}()
	if i >= 0 {
		panic("recovered")
	}
	return i
}

func main() {
	for i := range 16 {
		if alwaysDefer(i) != i+2 || condDefer(i) != 2*i || loopDefer(i) != i+3 || recovered(i) != 2*i {
			println("value", i)
			os.Exit(2)
		}
	}
}
