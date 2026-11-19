//go:build !gopacabra || !gopacabramn || !(linux || darwin)

package runtime

func procShard() (uint32, bool) { return 0, false }
