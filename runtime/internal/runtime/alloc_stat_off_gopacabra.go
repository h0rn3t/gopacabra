//go:build gopacabra && !gopacabraasan && (linux || darwin) && !baremetal && !gopacabrastat

package runtime

const gopacabraAllocCFile = "_wrap/gopacabra_alloc.c"
