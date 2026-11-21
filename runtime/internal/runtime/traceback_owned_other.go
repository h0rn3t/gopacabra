//go:build !llgo || !linux || wasm

package runtime

func markTracebackOwned(*g) {}
