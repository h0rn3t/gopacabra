//go:build go1.26 && llgo

package runtime

// fipsBypassDepth is part of the calling goroutine's cryptographic state. It
// must follow that goroutine if a future scheduler moves it between OS threads.
//
//llgointernal:gls
var fipsBypassDepth uint32
