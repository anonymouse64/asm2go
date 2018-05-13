# asm2go

[![Go Report Card](https://goreportcard.com/badge/github.com/anonymouse64/asm2go)](https://goreportcard.com/report/github.com/anonymouse64/asm2go)
[![license](https://img.shields.io/badge/license-GPLv3-blue.svg)](LICENSE)
[![Build Status](https://travis-ci.com/anonymouse64/asm2go.svg?branch=master)](https://travis-ci.com/anonymouse64/asm2go)
[![codecov](https://codecov.io/gh/anonymouse64/asm2go/branch/master/graph/badge.svg)](https://codecov.io/gh/anonymouse64/asm2go)


This project aims to automatically generate working Golang assembly from native assembly and a golang declaration file, mainly used for implementing performance-intensive complex functions in assembly. 

## Usage

`asm2go` requires 2 files, a native assembly file that assembles properly using `as` (the GNU assembler), and a Golang declaration file that contains signatures for the functions implemented in assembly. These names must match exactly, if a symbol in the assembly doesn't have a corresponding go function declaration the generation fails (this restriction is somewhat arbitrary right now and may be lifted in the future). 

Furthermore, the assembler must either be specified with the `-as` option, which can be a absolute path or a name on `$PATH`. In the same folder as the assembler must be the executables `strip` and `objdump` must also be available (note that specified a compiled with a prefix such as `arm-linux-gnueabihf-as` works; the prefix is resolved to find `arm-linux-gnueabihf-objdump`, etc - this allows cross compiling to work as expected). `strip` is used to remove debugging information from the compiled object file, and `objdump` is used to parse the actual hex instructions that are associated with instructions.

Assembler options may be specified with `as-opts`, as many times as needed. For example to use the options `-march=armv7-a` and the option `-mfpu=neon-vfpv4`, you would invoke `asm2go` as follows:

```
asm2go -file somefile.s -gofile somefile.go -as-opts -march=armv7-a -as-opts -mfpu=neon-vfpv4
```

The output can either be a file specified with the `-out` option, or if not specified the output is dumped to stdout.

#### Usage message

```
$ asm2go --help
Usage of asm2go:
  -as string
    	assembler to use (default "gas")
  -as-opts value
    	Assembler options to use
  -file string
    	file to assemble
  -gofile string
    	go file with function declarations
  -out string
    	output file to place data in (empty uses stdout)
```

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


## Limitations

1. Constant arrays are not supported, i.e. a label with data after it inside assembly does not work because when assembled, the references to addresses are usually absolute and the golang linker will move the constants around in memory. This may or may not be supported in when assembling/compiling from C/C++ code
	1. Constants could actually be supported per architecture by analyzing the instructions themselves, looking for instructions that use memory addresses in their arguments, checking what address is referred to by that instruction, then going and checking that address in the compiled symbols to see if that address points to the start of a symbol we have. If that's the case then it could be dynamically changed to a golang assembly directive doing the same instruction (if that's possible) with a global data reference, and then also resolve the global data symbol to be a DATA directive in the go code instead of a TEXT symbol like currently handled
2. Currently the frame size and argument sizes are both set to 0, I am hoping to fix this in the future to dynamically resolve this information from the signature. It is up to you of course to ensure your assembler function actually works with the specified frame/argument sizes. 
3. Only GAS is supported, I hope to also at least support `armcc` and `yasm` at some point. Windows assemblers are probably supported, though I've never worked with an equivalent to objdump on windows, so that step may prove difficult
4. No assembler function flags are specified currently, I think the best way to handle this is to check for comments in the doc comment, i.e. `// asm2go:nosplit` would add the flag `NOSPLIT` to the assembler declaration.

## License

This project is licensed under GPLv3. See LICENSE file for full license.
Copyright 2018 Canonical Ltd.