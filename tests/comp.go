package main

import (
	"fmt"
)

func myFunc(s ...any) {
	fmt.Println(s...)
}

func main() {
	ds := []int{}
	__t1 := 1
	for __t1 <= 10 {
		ds = append(ds, __t1)
		__t1 = __t1 + 1
	}
	fmt.Println(ds)
	evens := []int{}
	__t2 := 2
	for __t2 <= 8 {
		evens = append(evens, __t2)
		__t2 = __t2 + 1
	}
	__t3 := 0
	for __t3 < len(evens) {
		x := evens[__t3]
		fmt.Println(x)
		__t3 = __t3 + 1
	}
	fs := []float64{}
	__t4 := 1.0
	for __t4 <= 3.5 {
		fs = append(fs, __t4)
		__t4 = __t4 + 1
	}
	fmt.Println(fs)
	ms := []int{}
	__t5 := 0
	for __t5 < len(ds) {
		x := ds[__t5]
		if x%3 == 0 {
			if x > 5 {
				ms = append(ms, x*2)
			}
		}
		__t5 = __t5 + 1
	}
	fmt.Println(ms)
	myFunc(1, 2, 3.4, 5, 6, 7.8, 9, 10)
}
