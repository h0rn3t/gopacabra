//go:build gopacabra

// Програма прийняття P12.5.6.2: census розставляє місця функцій оверлея
// (runtime/internal/lib/...). reflect.unpackEface бере адресу параметра
// (`&i`), і параметр тікає в купу. Функції оверлея census не бачив, тож місце
// лишалося unplaced: 16 байт без звільнення на кожен reflect.ValueOf. Тест
// збирає програму з gopacabrastat і шукає unplaced-рядки файлів оверлея.
package main

import (
	"os"
	"reflect"
)

const rounds = 1000

type point struct{ x, y int }

func main() {
	p := &point{1, 2}
	sum := 0
	for i := range rounds {
		sum += int(reflect.ValueOf(i).Int())
		sum += reflect.ValueOf("abc").Len()
		sum += int(reflect.ValueOf(p).Elem().Field(1).Int())
		sum += int(reflect.ValueOf(point{3, 4}).Field(0).Int())
	}
	if want := rounds*(rounds-1)/2 + rounds*(3+2+3); sum != want {
		println("sum", sum, "want", want)
		os.Exit(1)
	}
}
