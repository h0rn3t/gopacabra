package main

// Програма для TestNoTinyGoGC: алокації (зріз, мапа) і defer — те, що
// раніше обслуговував tinygogc на wasm і baremetal.

func fill(n int) (sum int) {
	defer func() { sum++ }()
	s := make([]int, 0, 1)
	for i := 0; i < n; i++ {
		s = append(s, i*i)
	}
	m := map[int]int{}
	for i, v := range s {
		m[i] = v
	}
	for _, v := range m {
		sum += v
	}
	return sum
}

func main() {
	println("sum", fill(10))
}
