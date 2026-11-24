//go:build gopacabra

// Програма прийняття P12.5.5.1: комірка змінної-вказівника на циклічний тип,
// яку захоплює замикання, передане у виклик, звільняється при виході з
// функції. Форма — з database/sql: `db.retry(func(...) error { ... db.query
// ... })` у QueryContext і `withLock(dc, func() { ... dc.ci ... })` у queryDC.
// До виправлення комірка параметра типу *DB мала позначку cyclic: closure і
// лишалась у live cyclic на кожен виклик.
package main

import (
	"os"
	_ "unsafe"
)

//go:linkname gopacabraLive github.com/xgo-dev/llgo/runtime/internal/runtime.Live
func gopacabraLive(cat int32) uint64

const (
	catRCLocal = 1
	catCyclic  = 3
	n          = 64
)

// db і conn посилаються одне на одного, як sql.DB і sql.driverConn.
type db struct {
	conn    *conn
	retries int
}

type conn struct {
	db     *db
	locked int
	sum    int
}

func withLock(c *conn, fn func()) {
	c.locked++
	fn()
	c.locked--
}

func (d *db) retry(fn func(strategy int) error) error {
	d.retries++
	return fn(d.retries)
}

func (d *db) query(q, strategy int) (int, error) {
	return d.queryDC(d.conn, q+strategy-strategy)
}

// queryContext — форма DB.QueryContext: замикання захоплює параметр d.
func (d *db) queryContext(q int) (int, error) {
	var rows int
	var err error
	err = d.retry(func(strategy int) error {
		rows, err = d.query(q, strategy)
		return err
	})
	return rows, err
}

// queryDC — форма DB.queryDC: замикання для withLock захоплює параметр dc.
func (d *db) queryDC(dc *conn, q int) (int, error) {
	var rows int
	var err error
	withLock(dc, func() {
		if dc.locked != 1 {
			rows = -1
			return
		}
		dc.sum += q
		rows = q
	})
	return rows, err
}

func main() {
	d := &db{}
	d.conn = &conn{db: d}
	local, ring := gopacabraLive(catRCLocal), gopacabraLive(catCyclic)
	got := 0
	for i := range n {
		rows, err := d.queryContext(i)
		if err != nil {
			os.Exit(2)
		}
		got += rows
	}
	if want := n * (n - 1) / 2; got != want || d.conn.sum != want || d.retries != n {
		println("sum", got, d.conn.sum, "retries", d.retries, "want", want, n)
		os.Exit(3)
	}
	if delta := gopacabraLive(catCyclic) - ring; delta != 0 {
		println("live cyclic delta", delta, "want 0")
		os.Exit(4)
	}
	if delta := gopacabraLive(catRCLocal) - local; delta != 0 {
		println("live rc-local delta", delta, "want 0")
		os.Exit(5)
	}
	os.Exit(0)
}
