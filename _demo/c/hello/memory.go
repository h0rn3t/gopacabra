package main

import "github.com/goplus/lib/c"

// verifyCMemory exercises the libc data path: malloc, memset, strncpy and free.
func verifyCMemory() {
	buffer := c.Malloc(6)
	if buffer == nil {
		panic("C malloc")
	}
	defer c.Free(buffer)

	c.Memset(buffer, 0, 6)
	c.Strncpy((*c.Char)(buffer), c.Str("abcde"), 5)
	if c.Strcmp((*c.Char)(buffer), c.Str("abcde")) != 0 || byte(c.Index((*c.Char)(buffer), 0)) != 'a' {
		panic("C string copy")
	}

	c.Memset(buffer, c.Int('A'), 5)
	if c.Strcmp((*c.Char)(buffer), c.Str("AAAAA")) != 0 {
		panic("C memset")
	}
}
