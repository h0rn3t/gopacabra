//go:build gopacabra && gopacabramn && gopacabraasan && (linux || darwin)

package runtime

import "unsafe"

func bindAllocatorCache(*unsafe.Pointer) {}

func unbindAllocatorCache() {}

// MNAllocHeapsForTesting — під ASan купи алокатора gopacabra немає.
func MNAllocHeapsForTesting() int { return 0 }
