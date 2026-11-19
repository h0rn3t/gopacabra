//go:build !(gopacabra && gopacabramn && (linux || darwin))

package runtime

func gomaxprocs(int) int { return int(c_maxprocs()) }
