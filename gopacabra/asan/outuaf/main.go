//go:build gopacabra

// Програма прийняття P4.3: читання після FreeAccounted у пакеті поза main
// має завершитись звітом ASan.
package main

import "github.com/xgo-dev/llgo/gopacabra/asan/outuaf/site"

func main() {
	site.ReadAfterFree()
}
