// asmfuncdata перевіряє асемблерну функцію з макросами funcdata.h,
// яка кличе Go-функцію через стек (як ccgo у modernc.org/libc).
package main

import "fmt"

func callAdd(a, b int64) int64

func add(a, b int64) int64 { return a*10 + b }

func main() {
	fmt.Println(callAdd(4, 2))
	fmt.Println(callAdd(-3, 7))
}
