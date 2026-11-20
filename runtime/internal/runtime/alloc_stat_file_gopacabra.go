//go:build gopacabra && !gopacabraasan && (linux || darwin) && !baremetal && gopacabrastat && !(gopacabraheap && linux)

package runtime

// З тегом gopacabrastat алокатор збирається з лічильниками (P-NEW-08.1).
// Семплер купи gopacabraheap — лише Linux (/proc/self/maps), на macOS тег
// нічого не додає.
const gopacabraAllocCFile = "_wrap/gopacabra_alloc_stat.c"
