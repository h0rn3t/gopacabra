//go:build (amd64 || arm64 || wasm) && !(linux && arm64)

package cpu

func llgoPrepareCPU() {}
