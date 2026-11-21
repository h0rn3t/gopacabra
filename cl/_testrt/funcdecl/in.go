// LITTEST
package main

import (
	"unsafe"
)

// A declared function is represented as a closure pair with a nil environment;
// in an interface its data word is the constant global {fn, nil}, as gc keeps
// the funcval of a static function, and interface assertions use the closure
// type descriptor.
// CHECK: @"main.demo$funcval" = linkonce_odr constant { ptr, ptr } { ptr @main.demo, ptr null }
// CHECK-LABEL: define void @main.check({ ptr, ptr } %0){{.*}} {
// CHECK: %[[ARG_BOX:[0-9]+]] = call ptr @"{{.*}}/runtime/internal/runtime.AllocU"(i64 16)
// CHECK-NEXT: store { ptr, ptr } %0, ptr %[[ARG_BOX]]
// CHECK-NEXT: %[[ARG_EFACE:[0-9]+]] = insertvalue %"{{.*}}runtime.eface" { ptr @[[CLOSURE_TYPE:"_llgo_closure\$[^"]+"]], ptr undef }, ptr %[[ARG_BOX]], 1
// CHECK-NEXT: %[[DECL_MATCH:[0-9]+]] = call i1 @"{{.*}}/runtime/internal/runtime.MatchesClosure"(ptr @[[CLOSURE_TYPE]], ptr @[[CLOSURE_TYPE]])
// CHECK: %[[ARG_TYPE:[0-9]+]] = extractvalue %"{{.*}}runtime.eface" %[[ARG_EFACE]], 0
// CHECK-NEXT: %[[ARG_MATCH:[0-9]+]] = call i1 @"{{.*}}/runtime/internal/runtime.MatchesClosure"(ptr @[[CLOSURE_TYPE]], ptr %[[ARG_TYPE]])
// CHECK: %[[DECL_PTR:[0-9]+]] = call ptr @main.closurePtr(%"{{.*}}runtime.eface" { ptr @[[CLOSURE_TYPE]], ptr @"main.demo$funcval" })
// CHECK-NEXT: %[[ARG_PTR:[0-9]+]] = call ptr @main.closurePtr(%"{{.*}}runtime.eface" %[[ARG_EFACE]])
// CHECK-NEXT: %[[SAME_PTR:[0-9]+]] = icmp eq ptr %[[DECL_PTR]], %[[ARG_PTR]]
// CHECK-NEXT: call void @"{{.*}}/runtime/internal/runtime.PrintBool"(i1 %[[SAME_PTR]])
// CHECK-LABEL: define ptr @main.closurePtr(%"{{.*}}runtime.eface" %0){{.*}} {
// The address of a is only reinterpreted and read through a field, so the
// copy of the interface stays on the stack.
// CHECK: %[[EFACE_BOX:[0-9]+]] = alloca %"{{.*}}runtime.eface"
// CHECK-NOT: AllocZ
// CHECK: store %"{{.*}}runtime.eface" %0, ptr %[[EFACE_BOX]]
// CHECK: %[[CLOSURE_SLOT:[0-9]+]] = getelementptr inbounds nuw %main.rtype, ptr %[[EFACE_BOX]], i32 0, i32 1
// CHECK-NEXT: %[[CLOSURE:[0-9]+]] = load ptr, ptr %[[CLOSURE_SLOT]]
// CHECK-NEXT: %[[CODE_SLOT:[0-9]+]] = getelementptr inbounds nuw { ptr, ptr }, ptr %[[CLOSURE]], i32 0, i32 0
// CHECK-NEXT: %[[CODE:[0-9]+]] = load ptr, ptr %[[CODE_SLOT]]
// CHECK-NEXT: ret ptr %[[CODE]]
// CHECK-LABEL: define void @main.demo(){{.*}} {
// CHECK: call void @"{{.*}}/runtime/internal/runtime.PrintString"(%"{{.*}}runtime.String" { ptr @{{[0-9]+}}, i64 4 })
// CHECK: ret void
// CHECK-LABEL: define void @main.main(){{.*}} {
// CHECK: call void @main.check({ ptr, ptr } { ptr @main.demo, ptr null })

func check(fn func()) {
	var a any = demo
	var b any = fn
	fn1 := a.(func())
	fn2 := b.(func())
	println(a, b, fn, fn1, fn2, demo)
	println(closurePtr(a) == closurePtr(b))
}

func closurePtr(a any) unsafe.Pointer {
	return (*rtype)(unsafe.Pointer(&a)).ptr.fn
}

type rtype struct {
	typ unsafe.Pointer
	ptr *struct {
		fn  unsafe.Pointer
		env unsafe.Pointer
	}
}

func demo() {
	println("demo")
}

func main() {
	println("hello")
	check(demo)
}
