//go:build gopacabra && gopacabraasan

package main

// checkAllocator нічого не перевіряє: під -asan блоки виділяє malloc ASan.
func checkAllocator() {}
