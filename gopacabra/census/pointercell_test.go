package census

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/ssa"
)

// database/sql.queryDC: замикання для withLock захоплює параметр dc типу
// *driverConn, а driverConn через db досягає себе. Комірка параметра — одне
// слово, вузлом кільця вона не є, тож позначка в неї rc-local (P12.5.5.1).
// Комірка лишається cyclic, коли тип тримає вказівник на саму комірку.
const pointerCellSrc = `package p

type db struct{ conn *conn }
type conn struct {
	db *db
	n  int
}

type node struct{ self **node }

func withLock(c *conn, f func()) { f() }
func run(f func())               { f() }

func locked(c *conn) int {
	n := 0
	withLock(c, func() { n += c.n })
	return n
}

func ring(p *node) { run(func() { p.self = &p }) }
`

func TestPointerCellIsNotCyclicNode(t *testing.T) {
	pkg := buildOnePackage(t, "example.com/p", pointerCellSrc, ssa.SanityCheckFunctions)
	labels, _ := Sites(ssaFuncs(pkg))
	tests := []struct {
		fn, cell, head string
	}{
		{fn: "locked", cell: "c", head: "rc-local"},
		{fn: "ring", cell: "p", head: "cyclic"},
	}
	for _, tt := range tests {
		found := false
		for site, label := range labels {
			alloc, ok := site.(*ssa.Alloc)
			if !ok || alloc.Parent().Name() != tt.fn || alloc.Comment != tt.cell {
				continue
			}
			found = true
			if head, _, _ := strings.Cut(label, ":"); head != tt.head {
				t.Errorf("%s: cell %s label = %q, want head %q", tt.fn, tt.cell, label, tt.head)
			}
		}
		if !found {
			t.Errorf("%s: cell %s has no site", tt.fn, tt.cell)
		}
	}
}
