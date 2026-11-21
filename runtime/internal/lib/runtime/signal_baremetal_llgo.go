//go:build wasm

package runtime

// Wasm targets do not provide POSIX signal delivery.

func signal_enable(sig uint32) {
	_ = sig
}

func signal_disable(sig uint32) {
	_ = sig
}

func signal_ignore(sig uint32) {
	_ = sig
}

func signal_ignored(sig uint32) bool {
	_ = sig
	return false
}

func signalWaitUntilIdle() {}
