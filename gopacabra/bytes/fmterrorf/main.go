//go:build gopacabra

// Програма прийняття P12.5.7.3: форма database/sql.convertAssignRows.
// assign пише в dest відомого типу, а про решту повідомляє через
// fmt.Errorf("… %T …", src, dest). %T читає лише тип аргументу, тож dest
// позичений, і змінна v викликача звільняється на кожній ітерації. До
// правила census вважав параметр невідомим (друк fmt), і v лишалась живою.
// Звільнення місця звіряє TestGopacabraFmtErrorf за рядками gopacabra site.
package main

import (
	"fmt"
	"os"
)

const n = 1000

func assign(dest, src any) error {
	switch d := dest.(type) {
	case *int64:
		if s, ok := src.(int64); ok {
			*d = s
			return nil
		}
	}
	return fmt.Errorf("unsupported Scan, storing driver.Value type %T into type %T", src, dest)
}

func main() {
	var sum int64
	for i := range n {
		var v int64
		if err := assign(&v, int64(i)); err != nil {
			println(err.Error())
			os.Exit(2)
		}
		sum += v
	}
	var s string
	err := assign(&s, int64(1))
	if want := "unsupported Scan, storing driver.Value type int64 into type *string"; err == nil || err.Error() != want {
		println("assign(*string) =", err, "want", want)
		os.Exit(2)
	}
	if sum != n*(n-1)/2 {
		println("sum", sum)
		os.Exit(2)
	}
}
