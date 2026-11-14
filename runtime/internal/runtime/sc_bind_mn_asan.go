//go:build owngo && owngomn && owngoasan && (linux || darwin)

package runtime

import "unsafe"

func bindAllocatorCache(*unsafe.Pointer) {}

func unbindAllocatorCache() {}

// MNAllocHeapsForTesting — під ASan купи алокатора owngo немає.
func MNAllocHeapsForTesting() int { return 0 }
