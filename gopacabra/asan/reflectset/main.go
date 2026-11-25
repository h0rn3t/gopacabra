//go:build gopacabra

// Фікстура P12.5.7.2: reflect.TypeOf, Value.Set і SetZero позичають
// аргументи, але записане через Set живе в цілі, а тип, який збудувала
// рефлексія, живе в її кеші. Під ASan процес має завершитись без звіту.
package main

import (
	"os"
	"reflect"
)

type item struct {
	ID  int64
	Ref *int
	Any any
}

func size(args ...any) uintptr {
	var n uintptr
	for _, v := range args {
		n += reflect.TypeOf(v).Size()
	}
	return n
}

func set(dest, src any) {
	dv := reflect.Indirect(reflect.ValueOf(dest))
	sv := reflect.ValueOf(src)
	if sv.Type().AssignableTo(dv.Type()) {
		dv.Set(sv)
		return
	}
	dv.SetZero()
}

func main() {
	var items []*item
	for i := range 100 {
		if size(int64(i), int32(i), "x") != 8+4+16 {
			os.Exit(1)
		}
		it := new(item)
		set(&it.ID, int64(i))
		ref := new(int)
		*ref = i
		set(&it.Ref, "wrong type")
		set(&it.Ref, ref)
		set(&it.Any, &item{ID: int64(i)})
		items = append(items, it)
	}
	var sum int64
	for i, it := range items {
		if *it.Ref != i || it.Any.(*item).ID != int64(i) {
			os.Exit(2)
		}
		sum += it.ID
	}
	if sum != 99*100/2 {
		os.Exit(3)
	}

	pair := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(int64(0))},
		{Name: "B", Type: reflect.TypeOf("")},
	})
	for range 3 {
		v := reflect.New(pair).Elem()
		v.Field(0).SetInt(7)
		v.Field(1).SetString("seven")
		if reflect.TypeOf(v.Interface()) != pair || size(v.Interface()) != pair.Size() {
			os.Exit(4)
		}
	}
}
