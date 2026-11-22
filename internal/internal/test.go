package main

import "fmt"

func apply(f func(int) int, loop int) int {
	n := 0
	for range loop {
		n += f(n)
	}
	return n
}

func main() {
	base := 10
	n := apply(
		func(x int) int {
			return x + base
		},
		5,
	)

	fmt.Println("n = f(x) =", n)
}
