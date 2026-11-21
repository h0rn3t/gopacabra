//go:build gopacabra && !gopacabraasan && (linux || darwin) && !gopacabrastat

package runtime

const gopacabraAllocCFile = "_wrap/gopacabra_alloc.c"
