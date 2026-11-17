//go:build owngo

// Фікстура P11.5.0.3: path.Join будує string(buf) і повертає path.Clean
// від нього. Clean кладе параметр у поле локальної lazybuf і повертає
// підрядок цього поля, тож результат Join тримає string(buf). До
// виправлення census звільняв рядок одразу після Clean (embed.FS.ReadFile у
// fs.WalkDir читав звільнену пам'ять). Під ASan процес має завершитись без
// звіту.
package main

import (
	"os"
	"path"
)

func main() {
	a := path.Join("tree", "a.txt")
	b := path.Join("tree", "nested")
	if a != "tree/a.txt" || b != "tree/nested" {
		println("Join", a, b)
		os.Exit(1)
	}
}
