package cl_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/xgo-dev/llgo/cl/cltest"
	llssa "github.com/xgo-dev/llgo/ssa"
)

func TestCompileCooperativeSafepoints(t *testing.T) {
	const src = `package main

func leaf(p *int) *int {
	return p
}

func loop(p *int, n int) *int {
	for n > 0 {
		n--
	}
	return p
}

//go:nosplit
func noPoll(p *int) *int {
	return p
}
`
	ir := cltest.CompileIREx(t, src, "safepoint.go", false, func(prog llssa.Program) {
		prog.EnableGCRoots(true)
		prog.EnableCooperativeSafepoints(true)
	})

	leaf := findLLVMFunction(t, ir, "leaf")
	if got := strings.Count(leaf, "CooperativeSafepoint"); got != 1 {
		t.Fatalf("leaf has %d safepoints, want 1:\n%s", got, leaf)
	}
	if !strings.Contains(leaf, `[1 x ptr]`) {
		t.Fatalf("leaf parameter is not rooted across the entry safepoint:\n%s", leaf)
	}

	loop := findLLVMFunction(t, ir, "loop")
	if got := strings.Count(loop, "CooperativeSafepoint"); got != 2 {
		t.Fatalf("loop has %d safepoints, want entry plus backedge:\n%s", got, loop)
	}
	if !strings.Contains(loop, `[1 x ptr]`) {
		t.Fatalf("loop parameter is not rooted across safepoints:\n%s", loop)
	}

	noPoll := findLLVMFunction(t, ir, "noPoll")
	if strings.Contains(noPoll, "CooperativeSafepoint") ||
		strings.Contains(noPoll, "llvm_gc_root_chain") {
		t.Fatalf("//go:nosplit function contains a safepoint or root frame:\n%s", noPoll)
	}
}

// TestCompilePreemptRequestPoll: під M:N (P-NEW-09.10) safepoint читає
// прапорець рантайму атомарно і кличе CooperativeSafepoint лише за запиту
// витіснення, а без опитування цикл не мав би де віддати P.
func TestCompilePreemptRequestPoll(t *testing.T) {
	const src = `package main

func loop(n int) {
	for n > 0 {
		n--
	}
}

func bounded(a *[8]float64) {
	for k := range 8 {
		a[k] *= 2
	}
}

func recur(n int) int {
	if n == 0 {
		return 0
	}
	return recur(n-1) + 1
}
`
	ir := cltest.CompileIREx(t, src, "preempt_poll.go", false, func(prog llssa.Program) {
		prog.EnableCooperativeSafepoints(true)
		prog.EnablePreemptRequestPoll(true)
	})
	loop := findLLVMFunction(t, ir, "loop")
	loads := regexp.MustCompile(`load atomic i32, ptr @"[^"]*\.PreemptRequested" monotonic`)
	// Без викликів вхід не опитується: лишається зворотне ребро.
	if got := len(loads.FindAllString(loop, -1)); got != 1 {
		t.Fatalf("loop has %d monotonic PreemptRequested loads, want the backedge:\n%s", got, loop)
	}
	calls := regexp.MustCompile(`call void @"[^"]*\.CooperativeSafepoint"\(\)`)
	if got := len(calls.FindAllString(loop, -1)); got != 1 {
		t.Fatalf("loop has %d CooperativeSafepoint calls, want 1:\n%s", got, loop)
	}
	if got := strings.Count(loop, "icmp ne i32"); got < 1 {
		t.Fatalf("CooperativeSafepoint is not guarded by the request flag:\n%s", loop)
	}
	// Цикл range 8 скінченний, а викликів немає: опитувань немає зовсім.
	bounded := findLLVMFunction(t, ir, "bounded")
	if got := len(loads.FindAllString(bounded, -1)); got != 0 {
		t.Fatalf("bounded has %d PreemptRequested loads, want none:\n%s", got, bounded)
	}
	// Рекурсія без циклу поступається лише на вході.
	recur := findLLVMFunction(t, ir, "recur")
	if got := len(loads.FindAllString(recur, -1)); got != 1 {
		t.Fatalf("recur has %d PreemptRequested loads, want the entry:\n%s", got, recur)
	}
}

func TestCompileCooperativeSafepointsDisabled(t *testing.T) {
	const src = `package main
func loop(n int) {
	for n > 0 {
		n--
	}
}
`
	ir := cltest.CompileIREx(t, src, "safepoint_disabled.go", false, nil)
	if strings.Contains(ir, "CooperativeSafepoint") {
		t.Fatalf("disabled cooperative safepoints changed ordinary code:\n%s", ir)
	}
}

func findLLVMFunction(t *testing.T, ir, name string) string {
	t.Helper()
	pattern := regexp.MustCompile(`(?ms)^define [^{]*\.` + regexp.QuoteMeta(name) + `"?\([^)]*\).*?^\}`)
	body := pattern.FindString(ir)
	if body == "" {
		t.Fatalf("LLVM function %s not found:\n%s", name, ir)
	}
	return body
}
