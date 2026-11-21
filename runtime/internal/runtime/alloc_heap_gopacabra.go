//go:build gopacabra && !gopacabraasan && linux && gopacabrastat && gopacabraheap

package runtime

// З тегами gopacabrastat і gopacabraheap алокатор з лічильниками ще й семплює купу:
// стек кожного 64-го виділення і його звільнення, дамп у $GOPACABRA_HEAP_OUT
// на виході (P-NEW-09.3, gopacabra/bench/heapsym.py).
const gopacabraAllocCFile = "_wrap/gopacabra_alloc_heap.c"
