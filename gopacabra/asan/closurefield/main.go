//go:build gopacabra

// Фікстура P12.5.2: замикання, що захоплює параметр і лежить у полі чи йде
// з функції через return, викликають після повернення. Під ASan процес має
// завершитись без звіту: комірку не звільняють на шляху із замиканням.
package main

import "os"

type rowsT struct {
	cancel  func()
	lasterr int
	closed  int
}

func convertField(dest *int, src int, rows *rowsT) {
	if src < 0 {
		parentCancel := rows.cancel
		rows.cancel = func() {
			rows.closed += rows.lasterr
			if parentCancel != nil {
				parentCancel()
			}
		}
		return
	}
	*dest = src
}

func returnClosure(i int, sum *int) func() int {
	if i < 0 {
		return func() int { *sum += i; return i }
	}
	*sum += i
	return nil
}

// loopClosure — замикання створюють у циклі, комірка живе після виходу.
func loopClosure(rows *rowsT, k int) {
	for j := range k {
		if j == k-1 {
			rows.cancel = func() { rows.closed += j }
		}
	}
}

func main() {
	rows := &rowsT{}
	var dest, acc int
	for i := range 16 {
		convertField(&dest, i, rows)
		rows.lasterr = i
		convertField(&dest, -1-i, rows)
		if returnClosure(i, &acc) != nil || returnClosure(-1-i, &acc)() != -1-i {
			println("return", i)
			os.Exit(2)
		}
	}
	rows.cancel()
	if dest != 15 || rows.closed != 16*15 {
		println("dest", dest, "closed", rows.closed)
		os.Exit(3)
	}
	loopClosure(rows, 4)
	rows.closed = 0
	rows.cancel()
	if rows.closed != 3 {
		println("loop closed", rows.closed)
		os.Exit(4)
	}
}
