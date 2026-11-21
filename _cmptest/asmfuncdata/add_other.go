//go:build !amd64

package main

func callAdd(a, b int64) int64 { return add(a, b) }
