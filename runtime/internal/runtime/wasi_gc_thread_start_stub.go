//go:build !windows && (!llgo || !wasm || (wasip1 && llgo.wasi_threads))

package runtime

import "unsafe"

func wasiGCThreadStart(arg unsafe.Pointer) unsafe.Pointer { return mstart(arg) }
