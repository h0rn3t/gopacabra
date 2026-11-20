//go:build gopacabra && gopacabrastat && (gopacabraasan || !(linux || darwin) || baremetal)

package runtime

// Без алокатора gopacabra (ASan чи інша ОС) рядків gopacabra alloc немає.
func reportAlloc() {}
