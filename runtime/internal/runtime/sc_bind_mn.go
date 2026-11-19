//go:build gopacabra && gopacabramn && !gopacabraasan && (linux || darwin)

package runtime

import "unsafe"

//go:linkname gopacabraSCAttach C.gopacabra_sc_attach
func gopacabraSCAttach(held *unsafe.Pointer)

//go:linkname gopacabraSCDetach C.gopacabra_sc_detach
func gopacabraSCDetach()

func bindAllocatorCache(held *unsafe.Pointer) { gopacabraSCAttach(held) }

func unbindAllocatorCache() { gopacabraSCDetach() }

//go:linkname gopacabraSCHeapCount C.gopacabra_sc_heap_count
func gopacabraSCHeapCount() uintptr

// MNAllocHeapsForTesting — скільки куп алокатора створено з початку процесу.
func MNAllocHeapsForTesting() int { return int(gopacabraSCHeapCount()) }
