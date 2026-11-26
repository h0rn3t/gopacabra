//go:build gopacabra && gopacabrastat

// Програма прийняття P-NEW-03.6: лічильники покинутих масивів росту під тегом
// gopacabrastat. Три події: ріст зрізу через GrowSliceAligned зі старим
// cap > 0, завершення евакуації мапи (окремо зі знятим і встановленим
// oldIterator) і власницький append у межах ємності. Код 0 лише якщо зміна
// кожного лічильника між знімками дорівнює очікуваній.
package main

import (
	"os"
	_ "unsafe"
)

//go:linkname opCount github.com/xgo-dev/llgo/runtime/internal/runtime.OpCount
func opCount(op int32) uint64

// Номери збігаються з opGrowSlice … у runtime/internal/runtime.
const (
	opGrowSlice int32 = 21 + iota
	opGrowSliceBytes
	opMapGrowDone
	opMapGrowDoneBytes
	opMapGrowDoneIter
	opMapGrowDoneIterBytes
	opMapOldOverflow
	opOwnedAppendInCap
	opOwnedAppendInCapBytes
	opEnd
)

var names = [...]string{
	"grow_slice", "grow_slice_bytes",
	"map_grow_done", "map_grow_done_bytes",
	"map_grow_done_iter", "map_grow_done_iter_bytes",
	"map_old_overflow",
	"owned_append_in_cap", "owned_append_in_cap_bytes",
}

type snapshot [opEnd - opGrowSlice]uint64

func take() (s snapshot) {
	for i := range s {
		s[i] = opCount(opGrowSlice + int32(i))
	}
	return s
}

// check звіряє зміну лічильників з want; -1 у want — лічильник не звіряється.
func check(name string, code int, before snapshot, want [opEnd - opGrowSlice]int64) {
	after := take()
	for i, w := range want {
		if w < 0 {
			continue
		}
		if got := after[i] - before[i]; got != uint64(w) {
			println(name, names[i], "delta", got, "want", w)
			os.Exit(code)
		}
	}
}

// grown — зріз, що росте поза власницьким append: результат повертається.
// Ріст із cap 0 старого масиву не має і не рахується.
//
//go:noinline
func grown(n int) (s []int, grows, bytes int64) {
	for i := range n {
		c := cap(s)
		s = append(s, i)
		if cap(s) != c && c > 0 {
			grows++
			bytes += int64(c) * 8
		}
	}
	return s, grows, bytes
}

// owned — власницький append (emitOwnedAppend, як staticAppend у
// TestGopacabraImplicit): результат лише в len, census звільняє його буфер.
//
//go:noinline
func owned(s, extra []int) int {
	s = append(s, extra...)
	return len(s)
}

// bucketBytes — розмір бакета map[int]int: 8 tophash, 8 ключів, 8 значень,
// вказівник переповнення.
const bucketBytes = 8 + 8*8 + 8*8 + 8

// arrayBytes повторює makeBucketArray: від b = 4 масив має ще 2^(b-4)
// бакетів переповнення (roundupsize llgo розмір не змінює).
func arrayBytes(b uint) int64 {
	n := int64(1) << b
	if b >= 4 {
		n += int64(1) << (b - 4)
	}
	return n * bucketBytes
}

// mapKeys вставок у порожню мапу дають рости B 0→8: остання евакуація
// завершується задовго до 1000-ї вставки (кожна вставка евакуює щонайменше
// один старий бакет). Росту того самого розміру не буде: бакетів
// переповнення менше за 2^B, поки навантаження не перевищує 6.5.
const (
	mapKeys  = 1000
	mapGrows = 8
)

func mapGrowBytes(from, to uint) (sum int64) {
	for b := from; b < to; b++ {
		sum += arrayBytes(b)
	}
	return sum
}

//go:noinline
func fill(m map[int]int, from, to int) {
	for i := from; i < to; i++ {
		m[i] = i
	}
}

func main() {
	// (1) Ріст зрізу.
	before := take()
	s, grows, bytes := grown(1000)
	check("slice", 1, before, [...]int64{grows, bytes, 0, 0, 0, 0, 0, 0, 0})
	if len(s) != 1000 || grows == 0 {
		println("slice len", len(s), "grows", grows)
		os.Exit(2)
	}

	// (2) Ріст мапи без обходу. Слайс переповнення мапи без вказівників
	// росте через append рантайму, тож подія (1) тут не звіряється.
	before = take()
	m := make(map[int]int)
	fill(m, 0, mapKeys)
	check("map", 3, before, [...]int64{-1, -1, mapGrows, mapGrowBytes(0, mapGrows), 0, 0, -1, 0, 0})
	if got := opCount(opMapOldOverflow) - before[opMapOldOverflow-opGrowSlice]; got == 0 {
		println("map old overflow delta 0, want > 0")
		os.Exit(4)
	}
	if len(m) != mapKeys {
		os.Exit(5)
	}

	// Ріст під час range: перший ріст переносить iterator в oldIterator,
	// наступний його знімає, тож лише перша таблиця — з ітератором.
	before = take()
	r := make(map[int]int)
	fill(r, 0, 8)
	for range r {
		fill(r, 8, mapKeys)
		break
	}
	check("range", 6, before, [...]int64{-1, -1, mapGrows - 1, mapGrowBytes(1, mapGrows), 1, arrayBytes(0), -1, 0, 0})
	if len(r) != mapKeys {
		os.Exit(7)
	}

	// (3) Власницький append: n викликів у межах ємності копіюють по
	// oldLen елементів; виклик понад ємність не рахується.
	const n, oldLen = 10, 10
	in := make([]int, oldLen, 64)
	full := make([]int, 64)
	extra := []int{1}
	before = take()
	total := 0
	for range n {
		total += owned(in, extra)
	}
	total += owned(full, extra)
	check("owned", 8, before, [...]int64{-1, -1, 0, 0, 0, 0, 0, n, 8 * n * oldLen})
	if total != n*(oldLen+1)+65 {
		os.Exit(9)
	}
}
