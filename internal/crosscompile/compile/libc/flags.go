package libc

var _libcLDFlags = []string{
	"-nostdlib",
	"-ffunction-sections",
	"-fdata-sections",
}

var _libcCCFlags = []string{
	"-Oz",
	"-fno-builtin",
	// freestanding tells compiler this is a baremental project
	"-ffreestanding",
}
