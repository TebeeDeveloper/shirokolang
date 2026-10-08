package main

import (
	"fmt"
)

func main() {
	cong := func(a int, b int) int {
		return a + b
	}
	fmt.Println(cong(3, 4))
}
