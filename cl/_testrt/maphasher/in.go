// LITTEST
package main

// Map types name a key-specific hasher on every target, as gc does, instead
// of the generic typehash closure that re-dispatches on the key's type
// descriptor for every lookup. Keys that need the descriptor (interfaces,
// structs with padding or non-memory fields) keep typehash.

// CHECK-DAG: @"map[_llgo_uint32]_llgo_int" = {{.*}} { ptr @"{{.*}}/runtime/internal/runtime.memhash32", ptr null }
// CHECK-DAG: @"map[_llgo_string]_llgo_int" = {{.*}} { ptr @"{{.*}}/runtime/internal/runtime.strhash", ptr null }
// CHECK-DAG: @"map[_llgo_float64]_llgo_int" = {{.*}} { ptr @"{{.*}}/runtime/internal/runtime.f64hash", ptr null }
// CHECK-DAG: @"map{{\[\[}}3]_llgo_uint8]_llgo_int" = {{.*}} { ptr @_llgo_memhash.3, ptr null }
// CHECK-DAG: @"map[_llgo_any]_llgo_int" = {{.*}} { ptr @"{{.*}}/runtime/internal/runtime.typehash", ptr @_llgo_any }

func main() {
	u := map[uint32]int{1: 1}
	s := map[string]int{"a": 2}
	f := map[float64]int{0.5: 3}
	a := map[[3]byte]int{{1, 2, 3}: 4}
	i := map[any]int{1: 5}
	println(u[1], s["a"], f[0.5], a[[3]byte{1, 2, 3}], i[1])
}
