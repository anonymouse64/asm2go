# asm2go

[![Go Report Card](https://goreportcard.com/badge/github.com/anonymouse64/asm2go)](https://goreportcard.com/report/github.com/anonymouse64/asm2go)
[![license](https://img.shields.io/badge/license-GPLv3-blue.svg)](LICENSE)


This project aims to completely automatically generate working Golang assembly from native assembly and a golang declaration file (file with golang declarations of the assembly function - this is a normal go file, see examples).

1. Constant arrays are not supported, i.e. a label with data after it inside assembly does not work because when assembled, the references to addresses are absolute and the golang linker will move the constants around in memory. This may or may not be supported in when assembling/compiling from C/C++ code
1b. Constants could actually be supported per architecture by analyzing the instructions themselves, looking for instructions that use memory addresses in their arguments, checking what address is referred to by that instruction, then going and checking that address in the compiled symbols to see if that address points to the start of a symbol we have. If that's the case then it could be dynamically changed to a golang assembly directive doing the same instruction (if that's possible) with a global data reference, and then also resolve the global data symbol to be a DATA directive in the go code instead of a TEXT symbol like currently handled
2. A corresponding golang file with declarations must be passed in as well in order to generate appropriate / matching declarations

## Examples

### Keccak

This example uses the assembly file from the KeccakCodePackage here : https://github.com/gvanas/KeccakCodePackage. Specifically, the ARMv7A implementation here: https://github.com/gvanas/KeccakCodePackage/blob/master/lib/low/KeccakP-1600/OptimizedAsmARM/KeccakP-1600-armv7a-le-neon-gcc.s was modified to only contain the KeccakF-1600 function (it was also modified to use some constants passed in as an argument instead of hard-coded into the assembly as a symbol). 

To generate the native go assembly from the native ARM assembly copied here run:

	$ git clone github.com/anonymouse64/asm2go
	$ cd asm2go
	$ go install
	$ export PATH="$GOPATH/bin:$PATH"
	$ cd tests/keccak_arm
	$ go generate
	$ go build keccak_check.go
	$ ./keccak_check
	Success!
	$

This uses the go:generate comment inside `keccak_check.go`

The assembly generated uses the WORD feature of Plan9 assembly to insert all native instructions like such:

```
TEXT ·KeccakF1600(SB), 0, $0-8
    WORD $0xe1a0200e;  // mov      r2        lr 
    WORD $0xed2d8b10;  // vpush    {d8-d15}  
    WORD $0xf42007dd;  // vld1.64  {d0}      [r0 :64]!  
    WORD $0xf42027dd;  // vld1.64  {d2}      [r0 :64]!  
    WORD $0xf42047dd;  // vld1.64  {d4}      [r0 :64]!  
    WORD $0xf42067dd;  // vld1.64  {d6}      [r0 :64]!  
...
```

The only required parts are the working ARM assembly file (`keccak.s`) and the go declaration file which just declares the assembly-implemented function in go. The go file doesn't need anything extra, just the function declaration:

```
package keccak

// go:noescape
// This function is implemented in keccak.s
func KeccakF1600(state *[25]uint64, constants *[24]uint64)

```


## License

This project is licensed under GPLv3. See LICENSE file for full license.
Copyright 2018 Canonical Ltd.