//go:build gopacabra && gopacabrastat

package runtime

import (
	"unsafe"

	"github.com/xgo-dev/llgo/runtime/internal/sync/atomic"
)

// opShard — лічильники операцій потоків одного шарду обліку. 64 байти
// після лічильників відділяють останній від першого лічильника сусіднього
// шарду, тож шарди не ділять кеш-лінію.
type opShard struct {
	ops [opCount]uint64
	_   [64]byte
}

var opShards [accountShardCount]opShard

// opNames — назви рядків gopacabra ops у порядку номерів операцій.
var opNames = [opCount]string{
	"alloc", "alloc_root", "alloc_counted", "inc", "dec_accounted",
	"dec_cyclic", "cyclic_visit", "inc_shared", "dec_shared",
	"free_accounted", "free_root",
	"note_alloc_static", "note_alloc_rc-local", "note_alloc_rc-shared",
	"note_alloc_cyclic", "note_alloc_pinned", "note_alloc_implicit",
	"region_create", "alloc_region", "free_region", "dec_chan",
	"grow_slice", "grow_slice_bytes",
	"map_grow_done", "map_grow_done_bytes",
	"map_grow_done_iter", "map_grow_done_iter_bytes",
	"map_old_overflow",
	"owned_append_in_cap", "owned_append_in_cap_bytes",
}

func countOp(op int32) {
	addOp(op, 1)
}

func addOp(op int32, n uint64) {
	atomic.Add(&opShards[myShardIndex()].ops[op], n)
}

// noteGrowSlice рахує старий масив, який GrowSliceAligned лишає після росту.
func noteGrowSlice(oldCap, etSize int) {
	if oldCap == 0 {
		return
	}
	countOp(opGrowSlice)
	addOp(opGrowSliceBytes, uint64(oldCap)*uint64(etSize))
}

// oldBucketArray повертає початок і розмір у байтах старого масиву бакетів
// мапи, що росте: стільки ж бакетів, скільки дає makeBucketArray для старого B.
func oldBucketArray(t *maptype, h *hmap) (unsafe.Pointer, uintptr) {
	b := h.B
	if !h.sameSizeGrow() {
		b--
	}
	n := bucketShift(b)
	if b >= 4 {
		n += bucketShift(b - 4)
		sz := t.Bucket.Size_ * n
		if up := roundupsize(sz); up != sz {
			n = up / t.Bucket.Size_
		}
	}
	return h.oldbuckets, t.Bucket.Size_ * n
}

// noteEvacuate рахує бакети переповнення ланцюга старого бакета b, виділені
// поза масивом (newoverflow через newobject). Кличе evacuate до перенесення:
// після нього ланцюг може бути обнулений.
func noteEvacuate(t *maptype, h *hmap, b *bmap) {
	base, size := oldBucketArray(t, h)
	for ovf := b.overflow(t); ovf != nil; ovf = ovf.overflow(t) {
		if off := uintptr(unsafe.Pointer(ovf)) - uintptr(base); off >= size {
			countOp(opMapOldOverflow)
		}
	}
}

// noteGrowDone рахує старий масив бакетів, коли евакуація завершена, окремо
// для встановленого oldIterator: його ще може читати ітератор.
func noteGrowDone(t *maptype, h *hmap) {
	_, size := oldBucketArray(t, h)
	if h.flags&oldIterator != 0 {
		countOp(opMapGrowDoneIter)
		addOp(opMapGrowDoneIterBytes, uint64(size))
		return
	}
	countOp(opMapGrowDone)
	addOp(opMapGrowDoneBytes, uint64(size))
}

// OwnedAppendCapStat — OwnedAppendCap під gopacabrastat (cl, emitOwnedAppend):
// виклик з newLen ≤ oldCap рахує байти старого вмісту, які emitOwnedAppend
// копіює в новий буфер, хоча ємності вистачало.
func OwnedAppendCapStat(oldLen, newLen, oldCap, etSize int) int {
	if newLen >= 0 && newLen <= oldCap {
		countOp(opOwnedAppendInCap)
		addOp(opOwnedAppendInCapBytes, uint64(oldLen)*uint64(etSize))
	}
	return OwnedAppendCap(newLen, oldCap, etSize)
}

// OpCount повертає кількість викликів операції op за весь процес.
// Номер поза 0…opCount-1 дає 0.
func OpCount(op int32) uint64 {
	if op < 0 || op >= opCount {
		return 0
	}
	var n uint64
	for i := range opShards {
		n += atomic.Load(&opShards[i].ops[op])
	}
	return n
}

func reportOps() {
	for op, name := range opNames {
		println("gopacabra ops", name, OpCount(int32(op)))
	}
	reportAlloc()
	reportSites()
}

// siteRow і siteTable — розкладка таблиці gopacabra site, яку компілятор дає
// кожному пакету під gopacabrastat (cl.gopacabraSiteTable): лічильник і текст
// «file:line категорія причина операція».
type siteRow struct {
	n    *uint64
	text string
}

type siteTable struct {
	next *siteTable
	rows []siteRow
}

var siteTables *siteTable

// RegisterSites додає таблицю пакета до списку. Init пакета кличе його один
// раз; повторний виклик з тією самою таблицею нічого не змінює.
func RegisterSites(t *siteTable) {
	for s := siteTables; s != nil; s = s.next {
		if s == t {
			return
		}
	}
	t.next = siteTables
	siteTables = t
}

// siteCount — рядок gopacabra site зі знятим значенням лічильника.
type siteCount struct {
	n    uint64
	text string
}

// reportSites друкує рядки gopacabra site з ненульовим лічильником за спаданням
// n. Рядки з тим самим текстом з різних пакетів (екземпляри узагальнених
// функцій) зливаються в один. Буфер — AllocRoot, а не append: операція
// місця тут змінила б лічильники, які звіт саме читає.
func reportSites() {
	total := 0
	for t := siteTables; t != nil; t = t.next {
		total += len(t.rows)
	}
	if total == 0 {
		return
	}
	buf := AllocRoot(uintptr(total) * unsafe.Sizeof(siteCount{}))
	rows := unsafe.Slice((*siteCount)(buf), total)[:0]
	for t := siteTables; t != nil; t = t.next {
		for _, r := range t.rows {
			if n := atomic.Load(r.n); n != 0 {
				rows = rows[:len(rows)+1]
				rows[len(rows)-1] = siteCount{n, r.text}
			}
		}
	}
	sortSites(rows, func(a, b *siteCount) bool { return a.text < b.text })
	merged := rows[:0]
	for _, r := range rows {
		if last := len(merged) - 1; last >= 0 && merged[last].text == r.text {
			merged[last].n += r.n
			continue
		}
		merged = merged[:len(merged)+1]
		merged[len(merged)-1] = r
	}
	sortSites(merged, func(a, b *siteCount) bool {
		return a.n > b.n || a.n == b.n && a.text < b.text
	})
	for _, r := range merged {
		println("gopacabra site", r.text, r.n)
	}
	FreeRoot(buf)
}

// sortSites — сортування Шелла: пакет runtime не імпортує sort.
func sortSites(rows []siteCount, less func(a, b *siteCount) bool) {
	for gap := len(rows) / 2; gap > 0; gap /= 2 {
		for i := gap; i < len(rows); i++ {
			for j := i; j >= gap && less(&rows[j], &rows[j-gap]); j -= gap {
				rows[j], rows[j-gap] = rows[j-gap], rows[j]
			}
		}
	}
}
