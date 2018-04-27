package main

//go:generate asm2go -file src/addition.s -gofile addition/addition_amd64.go -out addition/addition_amd64.s

import (
	"fmt"

	"./addition"
)

func main() {
	fmt.Println(addition.Add2(2, 2))
}
