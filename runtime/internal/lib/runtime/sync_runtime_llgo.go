//go:build (darwin || linux || (llgo && wasip1 && wasm && llgo.wasi_threads)) && !(gopacabra && gopacabramn && (linux || darwin))

package runtime

import _ "unsafe"

var poolCleanup func()

//go:linkname sync_runtime_registerPoolCleanup sync.runtime_registerPoolCleanup
func sync_runtime_registerPoolCleanup(cleanup func()) {
	poolCleanup = cleanup
}
