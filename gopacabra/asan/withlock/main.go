//go:build gopacabra

// Фікстура P12.5.5.1: комірка змінної-вказівника на циклічний тип, яку
// захоплює замикання. Замикання для withLock не переживає виклик, решта —
// у полі, з return і в горутині — викликаються після повернення функції.
// Під ASan процес має завершитись без звіту: комірку звільняють лише там,
// де замикання не йде з функції.
package main

import "os"

type db struct {
	conn *conn
	hook func() int
}

type conn struct {
	db *db
	n  int
}

func withLock(c *conn, fn func()) { fn() }

// locked переписує захоплений параметр у замиканні й читає його після.
func locked(dc *conn) int {
	sum := 0
	withLock(dc, func() {
		sum += dc.n
		dc = dc.db.conn
	})
	return sum + dc.n
}

func kept(d *db, dc *conn)       { d.hook = func() int { return dc.n } }
func getter(dc *conn) func() int { return func() int { return dc.n } }
func spawn(dc *conn, ch chan int) {
	go func() { ch <- dc.n }()
}

func main() {
	d := &db{}
	d.conn = &conn{db: d, n: 3}
	ch := make(chan int)
	sum := 0
	for range 64 {
		sum += locked(d.conn)
		kept(d, d.conn)
		get := getter(d.conn)
		spawn(d.conn, ch)
		sum += d.hook() + get() + <-ch
	}
	if want := 64 * 15; sum != want {
		println("sum", sum, "want", want)
		os.Exit(2)
	}
	os.Exit(0)
}
