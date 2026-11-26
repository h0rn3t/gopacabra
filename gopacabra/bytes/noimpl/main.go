//go:build gopacabra

// Програма прийняття P12.5.7.4: форма database/sql (*Rows).scanLocked.
// dest іде в ScanColumn необов'язкового інтерфейсу драйвера, який у
// програмі не реалізує жоден тип: приведення не вдається ніколи, і виклик
// нічого не утримує. Тож dest позичений, і змінна v викликача звільняється
// на кожній ітерації. Звільнення місця звіряє TestGopacabraNoImplementer за
// рядками gopacabra site.
package main

import "os"

const n = 1000

type Rows interface{ Next() bool }

type ColumnScanner interface {
	ScanColumn(i int, dest any) error
}

type rows struct{ left int }

func (r *rows) Next() bool { r.left--; return r.left >= 0 }

func scan(r Rows, dest any) {
	if s, ok := r.(ColumnScanner); ok {
		if err := s.ScanColumn(0, dest); err != nil {
			panic(err)
		}
		return
	}
	if d, ok := dest.(*int64); ok {
		*d = 1
	}
}

func main() {
	r := &rows{left: n}
	var sum int64
	for r.Next() {
		var v int64
		scan(r, &v)
		sum += v
	}
	if sum != n {
		println("sum", sum)
		os.Exit(2)
	}
}
