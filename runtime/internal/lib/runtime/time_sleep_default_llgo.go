//go:build !gopacabra && !baremetal

package runtime

// Канал time.Sleep належить збирачу.
func freeSleepChan(chan struct{}) {}
