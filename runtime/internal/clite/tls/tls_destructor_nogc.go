//go:build llgo && !baremetal

package tls

func enterSlotDestructor() bool { return false }

func leaveSlotDestructor(bool) {}
