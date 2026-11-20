//go:build llgo && wasip1 && wasm && llgo.wasi_threads

package runtime

// CooperativeSafepoint is a no-op when the WASI pthread build disables GC.
func CooperativeSafepoint() {}
