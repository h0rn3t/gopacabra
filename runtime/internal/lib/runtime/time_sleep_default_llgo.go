//go:build !gopacabra

package runtime

// Канал time.Sleep належить збирачу.
func freeSleepChan(chan struct{}) {}
