package gcroot

var (
	currentRootChain uintptr
	sjljReplaying    bool
	activeContext    uintptr
	rebuilding       bool
)
