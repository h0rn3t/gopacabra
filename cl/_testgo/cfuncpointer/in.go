// LITTEST
// Scope: common
// cfuncpointer перевіряє слово значення функції, як cFuncPointer у
// modernc.org/sqlite: значення функції читають як uintptr з об'єкта, що з
// нього починається, а потім кличуть через це слово.
package main

// Слово значення функції, прочитане через unsafe.Pointer з об'єкта, що з
// нього починається, — вказівник на пару {fn, ctx} (runtime.FuncWord), а не
// перше слово пари: так його читає виклик через слово (ccgofp).

// CHECK-LABEL: define void @main.main(
// CHECK: call ptr @"{{.*}}/runtime/internal/runtime.FuncWord"(ptr %{{[0-9]+}}, ptr %{{[0-9]+}})

// CHECK-LABEL: define linkonce i64 @"main.cFuncPointer[func(a int32, b int32) int32]"(
// CHECK: load { ptr, ptr }, ptr %{{[0-9]+}}, align 8
// CHECK: call ptr @"{{.*}}/runtime/internal/runtime.FuncWord"(
// CHECK-NOT: load i64
// CHECK: ret i64

// Значення функції типу //llgo:cdecl в llgo — одне слово, сам код: його слово
// читається як є, без FuncWord, і виклик через слово бере код з нього.

// CHECK-LABEL: define i64 @main.cdeclWord(
// CHECK-NOT: FuncWord
// CHECK: ret i64

// CHECK-LABEL: define i64 @main.callCdecl(
// CHECK-NOT: load { ptr, ptr }
// CHECK: ret i64

import (
	"fmt"
	"runtime"
	"unsafe"
)

// cFuncPointer повторює modernc.org/sqlite.
func cFuncPointer[T any](f T) uintptr {
	return *(*uintptr)(unsafe.Pointer(&struct{ f T }{f}))
}

func add(a, b int32) int32 { return a + b }

//llgo:cdecl
type cAdd func(a, b int) int

func addInt(a, b int) int { return a + b }

func cdeclWord(c cAdd) uintptr {
	return *(*uintptr)(unsafe.Pointer(&c))
}

func callCdecl(w uintptr) int {
	return (*(*cAdd)(unsafe.Pointer(&struct{ uintptr }{w})))(2, 3)
}

func main() {
	fp := cFuncPointer(add)
	fmt.Println((*(*func(int32, int32) int32)(unsafe.Pointer(&struct{ uintptr }{fp})))(2, 3))
	fmt.Println(fp == cFuncPointer(add))
	f := add
	fmt.Println(*(*uintptr)(unsafe.Pointer(&f)) == fp)

	k := int32(7)
	mul := func(a int32) int32 { return a * k }
	cp := cFuncPointer(mul)
	fmt.Println((*(*func(int32) int32)(unsafe.Pointer(&cp)))(6))
	fmt.Println(cp == cFuncPointer(mul))
	runtime.KeepAlive(mul)

	fmt.Println(callCdecl(cdeclWord(addInt)))
}
