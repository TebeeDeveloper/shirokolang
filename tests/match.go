package main

import (
	"fmt"
)

func main() {
	x := 7
	switch {
	case x == 10:
		fmt.Println("ten")
	case 5 <= x && x <= 9:
		fmt.Println("range 5-9")
	case 1 <= x && x <= 4:
		fmt.Println("range 4-9")
	default:
		fmt.Println("other")
	}
	switch {
	case x == 1:
		fmt.Println("one")
	case x == 7:
		fmt.Println("seven")
	default:
		fmt.Println("nope")
	}
}
