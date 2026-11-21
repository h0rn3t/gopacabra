//go:build gopacabra && !gopacabraasan && (linux || darwin) && gopacabrastat

package runtime

import "unsafe"

// allocStatNames — назви рядків gopacabra alloc у порядку лічильників
// gopacabra_alloc_stat (enum SC_STAT_* у _wrap/gopacabra_alloc.c).
var allocStatNames = [...]string{
	"small", "large", "free_local", "free_remote", "free_large",
	"page_new", "page_reuse", "page_give", "madvise", "madvise_bytes", "lock",
}

//go:linkname gopacabraAllocStat C.gopacabra_alloc_stat
func gopacabraAllocStat(out unsafe.Pointer)

// allocStat — глобальна змінна, а не локальна: локальний масив під
// unsafe.Pointer став би алокацією pinned між рядками gopacabra ops і gopacabra
// site і розійшовся б з ними.
var allocStat [len(allocStatNames)]uint64

func reportAlloc() {
	gopacabraAllocStat(unsafe.Pointer(&allocStat[0]))
	for i, name := range allocStatNames {
		println("gopacabra alloc", name, allocStat[i])
	}
}
