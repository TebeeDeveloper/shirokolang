package main

func main() {
	__t1 := 0
	for __t1 < 5 {
		__t1 = __t1 + 1
	}
	i := 0
	for i < 5 {
		i = i + 1
	}
	ds := []int{1, 2, 3}
	__t2 := 0
	for __t2 < len(ds) {
		d := ds[__t2]
		_ = d
		__t2 = __t2 + 1
	}
	__t3 := 0
	for __t3 < len(ds) {
		i := __t3
		_ = i
		d := ds[__t3]
		_ = d
		__t3 = __t3 + 1
	}
}
