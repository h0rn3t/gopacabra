//go:build gopacabra

// Фікстура P11.5.0.3: internal/exportdata.lookupGorootExport кладе в
// sync.Map замикання, що захопило sync.Once, і викликає його через результат
// LoadOrStore. LoadOrStore лише зберігає параметр, тож census давав
// замиканню Inc, а захопленим коміркам — Dec на виході функції: повторний
// виклик замикання з мапи читав звільнену пам'ять («sync: unlock of
// unlocked mutex» у go/importer). Під ASan процес має завершитись без звіту.
package main

import (
	"os"
	"sync"
)

var m sync.Map

func get(k string) string {
	f, ok := m.Load(k)
	if !ok {
		var (
			once sync.Once
			v    string
		)
		f, _ = m.LoadOrStore(k, func() string {
			once.Do(func() { v = k + "!" })
			return v
		})
	}
	return f.(func() string)()
}

func main() {
	if a, b, c := get("a"), get("b"), get("a"); a != "a!" || b != "b!" || c != "a!" {
		println("get", a, b, c)
		os.Exit(1)
	}
}
