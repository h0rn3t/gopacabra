module github.com/xgo-dev/plan9asm

go 1.20

require github.com/xgo-dev/llvm v0.9.9

// Gopacabra: build against the LLVM 23 copy next to this one.
replace github.com/xgo-dev/llvm => ../llvm
