// LITTEST
// Scope: common
// ccgofp перевіряє виклик функції через слово даних інтерфейсу, як
// __ccgo_fp у modernc.org/libc: uintptr читають як значення функції через
// unsafe.Pointer з об'єкта розміру слова.
package main

// Значення функції, прочитане через unsafe.Pointer з об'єкта розміру слова,
// — це слово w, а під ним {fn, ctx}: так llgo кладе func в інтерфейс.
// Кожен із чотирьох викликів спершу читає слово, потім пару за ним.

// CHECK-LABEL: define void @main.main(
// CHECK: [[W1:%[0-9]+]] = load ptr, ptr %{{[0-9]+}}, align 8
// CHECK-NEXT: load { ptr, ptr }, ptr [[W1]], align 8
// CHECK: [[W2:%[0-9]+]] = load ptr, ptr %{{[0-9]+}}, align 8
// CHECK-NEXT: load { ptr, ptr }, ptr [[W2]], align 8
// CHECK: [[W3:%[0-9]+]] = load ptr, ptr %{{[0-9]+}}, align 8
// CHECK-NEXT: load { ptr, ptr }, ptr [[W3]], align 8
// CHECK: [[W4:%[0-9]+]] = load ptr, ptr %{{[0-9]+}}, align 8
// CHECK-NEXT: load { ptr, ptr }, ptr [[W4]], align 8

import (
	"fmt"
	"unsafe"
)

// keep тримає інтерфейси живими: далі програма зберігає лише uintptr.
var keep []interface{}

// ccgoFP повторює __ccgo_fp з modernc.org/libc.
func ccgoFP(f interface{}) uintptr {
	keep = append(keep, f)
	type iface [2]uintptr
	return (*iface)(unsafe.Pointer(&f))[1]
}

func add(a, b int32) int32 { return a + b }

func main() {
	fp := ccgoFP(add)
	fmt.Println((*(*func(int32, int32) int32)(unsafe.Pointer(&struct{ uintptr }{fp})))(2, 3))
	fmt.Println((*(*func(int32, int32) int32)(unsafe.Pointer(&fp)))(4, 5))

	k := int32(7)
	mul := func(a int32) int32 { return a * k }
	cp := ccgoFP(mul)
	fmt.Println((*(*func(int32) int32)(unsafe.Pointer(&struct{ uintptr }{cp})))(6))
	fmt.Println((*(*func(int32) int32)(unsafe.Pointer(&cp)))(8))
}
