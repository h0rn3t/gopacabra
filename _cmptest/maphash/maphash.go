package main

import (
	"fmt"
	"math"
	"strconv"
)

type pair struct{ A, B int64 }

type padded struct {
	A byte
	B int64
}

type blank struct {
	A int32
	_ int32
}

type stringer int

func (s stringer) String() string { return strconv.Itoa(int(s)) }

// count inserts n keys, grows the map past several evacuations, deletes every
// third key and reports what lookups of all n keys see.
func count[K comparable](n int, key func(int) K) string {
	m := map[K]int{}
	for i := range n {
		m[key(i)] = i
	}
	for i := 0; i < n; i += 3 {
		delete(m, key(i))
	}
	hits, sum := 0, 0
	for i := range n {
		if v, ok := m[key(i)]; ok {
			hits++
			sum += v
		}
	}
	return fmt.Sprint(len(m), hits, sum)
}

func main() {
	const n = 300
	fmt.Println("u8", count(256, func(i int) uint8 { return uint8(i) }))
	fmt.Println("u16", count(n, func(i int) uint16 { return uint16(i * 7) }))
	fmt.Println("u32", count(n, func(i int) uint32 { return uint32(i) << 20 }))
	fmt.Println("u64", count(n, func(i int) uint64 { return uint64(i) << 40 }))
	fmt.Println("16B", count(n, func(i int) pair { return pair{int64(i), -int64(i)} }))
	fmt.Println("3B", count(n, func(i int) [3]byte { return [3]byte{byte(i), byte(i >> 8), 1} }))
	fmt.Println("0B", count(n, func(i int) struct{} { return struct{}{} }))
	fmt.Println("str", count(n, func(i int) string { return fmt.Sprint("key-", i) }))
	fmt.Println("f32", count(n, func(i int) float32 { return float32(i) / 3 }))
	fmt.Println("c128", count(n, func(i int) complex128 { return complex(float64(i), -1) }))
	fmt.Println("padded", count(n, func(i int) padded { return padded{byte(i), int64(i)} }))
	fmt.Println("blank", count(n, func(i int) blank { return blank{A: int32(i)} }))
	fmt.Println("any", count(n, func(i int) any {
		if i%2 == 0 {
			return i
		}
		return strconv.Itoa(i)
	}))
	fmt.Println("iface", count(n, func(i int) fmt.Stringer { return stringer(i) }))

	// ±0 are one key; every NaN is a distinct key that no lookup finds.
	f := map[float64]int{}
	f[0] = 1
	f[math.Copysign(0, -1)] = 2
	f[math.NaN()] = 3
	f[math.NaN()] = 4
	_, nan := f[math.NaN()]
	sum := 0
	for k, v := range f {
		if k != k {
			sum += v
		}
	}
	fmt.Println("f64", len(f), f[0], nan, sum)
}
