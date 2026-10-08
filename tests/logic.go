package main

import (
	"fmt"
)

type __Set[K comparable] map[K]struct{}

func __setAdd[K comparable](s __Set[K], k K) __Set[K] {
	s[k] = struct{}{}
	return s
}

func __setHas[K comparable](s __Set[K], k K) bool {
	_, ok := s[k]
	return ok
}

func main() {
	a := 5
	b := 10
	fmt.Println(a > 0 && b > 0)
	fmt.Println(a > 0 || b < 0)
	fmt.Println(!a == b)
	fmt.Println(a != b)
}
