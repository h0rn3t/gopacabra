// Package fp дає функцію і її слово даних з іншого пакета: слово даних
// статичної функції однакове в усіх пакетах, як funcval у gc.
package fp

import "unsafe"

// Mul множить два числа.
func Mul(a, b int32) int32 { return a * b }

// MulFP повертає слово даних інтерфейсу з Mul, як __ccgo_fp.
func MulFP() uintptr {
	var f interface{} = Mul
	return (*[2]uintptr)(unsafe.Pointer(&f))[1]
}
