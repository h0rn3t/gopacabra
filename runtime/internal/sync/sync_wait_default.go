//go:build !windows

package sync

func mutexLock(m *Mutex) { pthreadMutexLock(m) }
