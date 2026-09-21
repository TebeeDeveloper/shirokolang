package cio

import (
	"os"
	"fmt"
	"strings"
)

type Read struct {
	Code string
	Err error
}

func ReadFile(input string) (read Read) {
	if !strings.HasSuffix(input, ".shrko") {
		read.Err = fmt.Errorf("File must have suffix with \".shrko\"")
		return
	}
	
	var b []byte
	var err error
	b, err = os.ReadFile(input)
	if err != nil {
		read.Err = err
		return
	}
	
	var cnt string = string(b)
	read.Code = cnt
	return
}
