/*
 * Copyright (c) 2026 The XGo Authors (xgo.dev). All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package safepointplan

import (
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// maxBoundedTrip — найбільше проходів зворотних ребер гнізда циклів, яке
// лишається без опитування: десятки мікросекунд проти кроку витіснення 10 мс.
const maxBoundedTrip = 4096

// maxLoopDepth обмежує глибину вкладених циклів, яку розбирає loopTrips.
const maxLoopDepth = 4

// boundedLimit обмежує сталі й значення лічильника: так 32-бітний тип не
// переповнюється за maxBoundedTrip кроків, і симуляція в int64 точна.
const boundedLimit = 1 << 30

// UnboundedBackedges — Backedges без гнізд циклів, чиї зворотні ребра разом
// проходяться не більше maxBoundedTrip разів: лічильник кожного циклу має
// сталі початок і крок, а умова виходу порівнює його зі сталою. Таке гніздо
// скінченне, і G поступиться в зовнішньому циклі чи на вході виклику, а
// опитування в ньому заважало б розгортанню й векторизації (P-NEW-09.10.1).
// Вкладені гнізда враховуються з запасом: межа кожного рахується й у межі
// гнізда, що його містить.
func UnboundedBackedges(fn *ssa.Function) map[ssa.Instruction]struct{} {
	polls := Backedges(fn)
	for instr := range polls {
		if closesBoundedLoop(instr.Block()) {
			delete(polls, instr)
		}
	}
	if len(polls) == 0 {
		return nil
	}
	return polls
}

// closesBoundedLoop каже, чи кожне ребро з latch, що замикає цикл, веде в
// заголовок обмеженого гнізда циклів.
func closesBoundedLoop(latch *ssa.BasicBlock) bool {
	var header *ssa.BasicBlock
	for _, succ := range latch.Succs {
		if succ.Dominates(latch) {
			if header != nil && header != succ {
				return false // ще й continue зовнішнього циклу
			}
			header = succ
		}
	}
	if header == nil {
		return false
	}
	for _, succ := range latch.Succs {
		// Інший наступник, що веде назад у latch повз заголовок, замикає
		// ще один цикл, можливо незвідний.
		if succ != header && reachesAvoiding(succ, latch, header) {
			return false
		}
	}
	_, ok := loopTrips(header, 0)
	return ok
}

// reachesAvoiding каже, чи з from є шлях у to, що не проходить через avoid.
func reachesAvoiding(from, to, avoid *ssa.BasicBlock) bool {
	if from == avoid {
		return false
	}
	seen := map[*ssa.BasicBlock]bool{from: true, avoid: true}
	work := []*ssa.BasicBlock{from}
	for len(work) != 0 {
		b := work[len(work)-1]
		work = work[:len(work)-1]
		if b == to {
			return true
		}
		for _, succ := range b.Succs {
			if !seen[succ] {
				seen[succ] = true
				work = append(work, succ)
			}
		}
	}
	return false
}

// loopTrips повертає верхню межу проходів зворотних ребер гнізда з
// заголовком header: власні ітерації, помножені на 1 плюс межі вкладених
// гнізд. Кожен цикл тіла мусить проходити через заголовок вкладеного гнізда,
// інакше (незвідний цикл) межі немає.
func loopTrips(header *ssa.BasicBlock, depth int) (int, bool) {
	if depth > maxLoopDepth {
		return 0, false
	}
	body, ok := loopBody(header)
	if !ok {
		return 0, false
	}
	headers := map[*ssa.BasicBlock]bool{header: true}
	for b := range body {
		for _, pred := range b.Preds {
			if b != header && body[pred] && b.Dominates(pred) {
				headers[b] = true
			}
		}
	}
	if cyclicWithout(body, headers) {
		return 0, false
	}
	inner := 1
	for nested := range headers {
		if nested == header {
			continue
		}
		trips, ok := loopTrips(nested, depth+1)
		if !ok {
			return 0, false
		}
		if inner += trips; inner > maxBoundedTrip {
			return 0, false
		}
	}
	own, ok := ownTrips(header, body)
	if !ok || own*inner > maxBoundedTrip {
		return 0, false
	}
	return own * inner, true
}

// ownTrips повертає найменшу межу ітерацій циклу header серед його
// лічильників.
func ownTrips(header *ssa.BasicBlock, body map[*ssa.BasicBlock]bool) (int, bool) {
	best, found := 0, false
	for _, instr := range header.Instrs {
		phi, ok := instr.(*ssa.Phi)
		if !ok {
			break
		}
		if trips, ok := inductionTrips(header, body, phi); ok && (!found || trips < best) {
			best, found = trips, true
		}
	}
	return best, found
}

// loopBody збирає блоки природного циклу header. false — тіло не
// підпорядковане заголовку.
func loopBody(header *ssa.BasicBlock) (map[*ssa.BasicBlock]bool, bool) {
	body := map[*ssa.BasicBlock]bool{header: true}
	var work []*ssa.BasicBlock
	for _, pred := range header.Preds {
		if header.Dominates(pred) && !body[pred] {
			body[pred] = true
			work = append(work, pred)
		}
	}
	for len(work) != 0 {
		b := work[len(work)-1]
		work = work[:len(work)-1]
		if !header.Dominates(b) {
			return nil, false
		}
		for _, pred := range b.Preds {
			if !body[pred] {
				body[pred] = true
				work = append(work, pred)
			}
		}
	}
	return body, true
}

// cyclicWithout каже, чи блоки body без headers мають цикл.
func cyclicWithout(body, headers map[*ssa.BasicBlock]bool) bool {
	const (
		unvisited uint8 = iota
		visiting
		visited
	)
	state := make(map[*ssa.BasicBlock]uint8, len(body))
	type frame struct {
		block *ssa.BasicBlock
		next  int
	}
	for root := range body {
		if headers[root] || state[root] != unvisited {
			continue
		}
		state[root] = visiting
		stack := []frame{{block: root}}
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if top.next == len(top.block.Succs) {
				state[top.block] = visited
				stack = stack[:len(stack)-1]
				continue
			}
			succ := top.block.Succs[top.next]
			top.next++
			if headers[succ] || !body[succ] {
				continue
			}
			switch state[succ] {
			case unvisited:
				state[succ] = visiting
				stack = append(stack, frame{block: succ})
			case visiting:
				return true
			default:
			}
		}
	}
	return false
}

// inductionTrips перевіряє phi як лічильник iv = c0, c0+step, …, де
// next = iv ± step приходить з кожного ребра тіла, і шукає вихід, який
// порівнює iv чи next зі сталою на кожній ітерації; повертає найменшу межу
// ітерацій серед таких виходів.
func inductionTrips(header *ssa.BasicBlock, body map[*ssa.BasicBlock]bool, phi *ssa.Phi) (int, bool) {
	basic, ok := phi.Type().Underlying().(*types.Basic)
	if !ok {
		return 0, false
	}
	switch basic.Kind() {
	case types.Int, types.Int32, types.Int64, types.Uint, types.Uint32, types.Uint64, types.Uintptr:
	default:
		return 0, false // вузькі типи переповнюються за менше кроків
	}
	unsigned := basic.Info()&types.IsUnsigned != 0

	var init, step int64
	var next ssa.Value
	haveInit := false
	for i, edge := range phi.Edges {
		if body[header.Preds[i]] {
			if next != nil && edge != next {
				return 0, false
			}
			next = edge
			continue
		}
		c, ok := smallConst(edge)
		if !ok || haveInit && c != init {
			return 0, false
		}
		init, haveInit = c, true
	}
	if next == nil || !haveInit {
		return 0, false
	}
	step, ok = stepOf(next, phi)
	if !ok {
		return 0, false
	}

	best, found := 0, false
	for b := range body {
		cond, ok := b.Instrs[len(b.Instrs)-1].(*ssa.If)
		if !ok || body[b.Succs[0]] == body[b.Succs[1]] {
			continue
		}
		// Вихід мусить перевірятися на кожній ітерації: b підпорядковує
		// кожен latch.
		dominatesLatches := true
		for _, pred := range header.Preds {
			if body[pred] && !b.Dominates(pred) {
				dominatesLatches = false
			}
		}
		if !dominatesLatches {
			continue
		}
		if trips, ok := tripCount(cond, phi, next, init, step, unsigned, body[b.Succs[0]]); ok && (!found || trips < best) {
			best, found = trips, true
		}
	}
	return best, found
}

// stepOf повертає крок, якщо next = phi + c чи phi - c.
func stepOf(next ssa.Value, phi *ssa.Phi) (int64, bool) {
	op, ok := next.(*ssa.BinOp)
	if !ok {
		return 0, false
	}
	var c int64
	switch {
	case op.X == phi:
		c, ok = smallConst(op.Y)
	case op.Y == phi && op.Op == token.ADD:
		c, ok = smallConst(op.X)
	default:
		return 0, false
	}
	if !ok || c == 0 {
		return 0, false
	}
	switch op.Op {
	case token.ADD:
		return c, true
	case token.SUB:
		return -c, true
	default:
		return 0, false
	}
}

// tripCount симулює умову виходу: стільки разів, скільки вона лишає
// керування в циклі, цикл щонайбільше й повторюється.
func tripCount(cond *ssa.If, phi *ssa.Phi, next ssa.Value, init, step int64, unsigned, trueStays bool) (int, bool) {
	cmp, ok := cond.Cond.(*ssa.BinOp)
	if !ok {
		return 0, false
	}
	op := cmp.Op
	var v ssa.Value
	var bound int64
	switch {
	case cmp.X == phi || cmp.X == next:
		v = cmp.X
		bound, ok = smallConst(cmp.Y)
	case cmp.Y == phi || cmp.Y == next:
		v = cmp.Y
		bound, ok = smallConst(cmp.X)
		op = swapCompare(op)
	default:
		return 0, false
	}
	if !ok {
		return 0, false
	}
	iv := init
	for trips := 0; trips <= maxBoundedTrip; trips++ {
		x := iv
		if v == next {
			x += step
		}
		if unsigned && x < 0 || x < -boundedLimit || x > boundedLimit {
			return 0, false // переповнення: поведінка вже не та, що в int64
		}
		holds, ok := compare(op, x, bound)
		if !ok {
			return 0, false
		}
		if holds != trueStays {
			return trips, true
		}
		iv += step
	}
	return 0, false
}

func compare(op token.Token, x, y int64) (bool, bool) {
	switch op {
	case token.EQL:
		return x == y, true
	case token.NEQ:
		return x != y, true
	case token.LSS:
		return x < y, true
	case token.LEQ:
		return x <= y, true
	case token.GTR:
		return x > y, true
	case token.GEQ:
		return x >= y, true
	default:
		return false, false
	}
}

func swapCompare(op token.Token) token.Token {
	switch op {
	case token.LSS:
		return token.GTR
	case token.LEQ:
		return token.GEQ
	case token.GTR:
		return token.LSS
	case token.GEQ:
		return token.LEQ
	default:
		return op // EQL і NEQ симетричні
	}
}

// smallConst повертає цілу сталу з модулем не більше boundedLimit.
func smallConst(v ssa.Value) (int64, bool) {
	c, ok := v.(*ssa.Const)
	if !ok || c.Value == nil || c.Value.Kind() != constant.Int {
		return 0, false
	}
	x, exact := constant.Int64Val(c.Value)
	if !exact || x < -boundedLimit || x > boundedLimit {
		return 0, false
	}
	return x, true
}

// NeedsEntryPoll каже, чи функції потрібне опитування на вході: так лише
// за виклику не вбудованої функції (зокрема рекурсії, go і defer), як
// перевірка стека в gc лише в нелистових функціях. Без викликів функція
// закінчується за скінченний час або поступається у своїх циклах.
func NeedsEntryPoll(fn *ssa.Function) bool {
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			call, ok := instr.(ssa.CallInstruction)
			if !ok {
				continue
			}
			if _, builtin := call.Common().Value.(*ssa.Builtin); !builtin {
				return true
			}
		}
	}
	return false
}
