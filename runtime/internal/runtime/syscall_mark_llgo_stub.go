//go:build llgo && !(gopacabra && gopacabramn && (linux || darwin))

package runtime

// Маркери викликів C (P-NEW-09.8) поза M:N: нічого не роблять. Компілятор
// ставить їх лише під gopacabramn, а runtime.entersyscall кличе завжди.

func EnterSyscall()            {}
func ExitSyscall()             {}
func CallbackEnter() uintptr   { return 0 }
func CallbackExit(tok uintptr) {}
