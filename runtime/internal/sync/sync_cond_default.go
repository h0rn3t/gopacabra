//go:build !windows && !(gopacabra && gopacabramn)

package sync

import c "github.com/xgo-dev/llgo/runtime/internal/clite"

// Waiting threads block in pthread_cond_wait directly; no collector needs them
// to enter a blocking scope.
func condWait(cond *Cond, mutex *Mutex) c.Int {
	return pthreadCondWait(cond, mutex)
}
