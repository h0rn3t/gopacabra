//go:build !((linux || darwin) && (amd64 || arm64)) && !windows

package setjmp

const (
	SigjmpBufSize = 200
	JmpBufSize    = 200
)
