package main

import (
	"fmt"
)

type GiaoDien interface {
	area() float64
}

type HinhVuong struct {
	dai int
}

func (self *HinhVuong) area() float64 {
	return float64(self.dai * self.dai)
}

func main() {
	hv := HinhVuong{dai: 5}
	fmt.Println(hv.area())
}
