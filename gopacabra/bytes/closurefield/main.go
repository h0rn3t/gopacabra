//go:build gopacabra

// Програма прийняття P12.5.2: комірка параметра, який захоплює замикання,
// що зберігається в полі, звільняється на шляхах, де замикання не
// створювали. Форма — з database/sql.convertAssignRows:
// `rows.cancel = func() { d.close(rows.lasterr); parentCancel() }` лише в
// гілці курсора. До виправлення кожен виклик лишав у live rc-local комірку.
package main

import (
	"os"
	_ "unsafe"
)

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

const (
	catRCLocal = 1
	n          = 64
)

type rowsT struct {
	cancel  func()
	lasterr int
	closed  int
}

var (
	parent  = &rowsT{}
	cursors int
)

// convertField — форма convertAssignRows: параметр rows захоплює замикання,
// яке кладуть у поле rows.cancel лише для курсора.
func convertField(dest *int, src int, rows *rowsT) int {
	if src < 0 {
		parentCancel := rows.cancel
		rows.cancel = func() {
			rows.closed += rows.lasterr
			if parentCancel != nil {
				parentCancel()
			}
		}
		cursors++
		return 0
	}
	*dest = src
	return 1
}

// returnClosure — замикання йде з функції через return лише в одній гілці.
// Тут перевіряються лише значення: комірки результату, що виходить з
// функції, exitSites не звільняє на жодному шляху.
func returnClosure(i int, sum *int) func() int {
	if i < 0 {
		return func() int { *sum += i; return i }
	}
	*sum += i
	return nil
}

// expect повертає код виходу: 0, якщо сума результатів і зміна live rc-local
// збіглися з очікуваними.
func expect(name string, f func(int) int, sum int, live uint64) int {
	before := gopacabraLive(catRCLocal)
	got := 0
	for i := range n {
		got += f(i)
	}
	if got != sum {
		println(name, "sum", got, "want", sum)
		return 2
	}
	if delta := gopacabraLive(catRCLocal) - before; delta != live {
		println(name, "live rc-local delta", delta, "want", live)
		return 4
	}
	return 0
}

func main() {
	var dest int
	field := func(i int) int { return convertField(&dest, i, parent) + dest }
	base := n * (n - 1) / 2
	if code := expect("field", field, base+n, 0); code != 0 {
		os.Exit(code)
	}
	// Шляхи з замиканням: комірка живе разом із ним, перевіряються значення.
	for i := range n {
		parent.lasterr = i
		convertField(&dest, -1-i, parent)
	}
	parent.cancel()
	if cursors != n || parent.closed != n*(n-1) {
		println("cursors", cursors, "closed", parent.closed)
		os.Exit(5)
	}
	acc := 0
	for i := range n {
		if f := returnClosure(-1-i, &acc); f == nil || f() != -1-i {
			println("closure", i)
			os.Exit(6)
		}
	}
	if acc != -base-n {
		println("acc", acc, "want", -base-n)
		os.Exit(7)
	}
	os.Exit(0)
}
