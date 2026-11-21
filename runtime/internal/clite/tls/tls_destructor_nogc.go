//go:build llgo

package tls

func enterSlotDestructor() bool { return false }

func leaveSlotDestructor(bool) {}
