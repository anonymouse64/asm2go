package main

//go:generate asm2go -file src/addition.s -gofile addition/addition_amd64.go -out addition/addition_amd64.s

import (
	"fmt"

	"github.com/anonymouse64/asm2go/tests/addition_amd64/addition"
)

func main() {
	fmt.Println(addition.Add2(2, 2))
}
