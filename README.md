# asm2go

[![Go Report Card](https://goreportcard.com/badge/github.com/anonymouse64/asm2go)](https://goreportcard.com/report/github.com/anonymouse64/asm2go)
[![license](https://img.shields.io/badge/license-GPLv3-blue.svg)](LICENSE)
[![Build Status](https://travis-ci.com/anonymouse64/asm2go.svg?branch=master)](https://travis-ci.com/anonymouse64/asm2go)
[![codecov](https://codecov.io/gh/anonymouse64/asm2go/branch/master/graph/badge.svg)](https://codecov.io/gh/anonymouse64/asm2go)


This project aims to automatically generate working Golang assembly from native assembly and a golang declaration file, mainly used for implementing performance-intensive complex functions in assembly. 

## Usage

`asm2go` requires 2 files, a native assembly file that assembles properly using `as` (the GNU assembler), and a Golang declaration file that contains signatures for the functions implemented in assembly. These names must match exactly, if a symbol in the assembly doesn't have a corresponding go function declaration the generation fails (this restriction is somewhat arbitrary right now and may be lifted in the future). 

As to writing the actual assembly code to be translated, there are a few caveats. 

0. Argument calling convention in Go places arguments on the stack, so you should write the assembly code to reference the stack for accessing arguments provided to functions. This may or may not match what is normally done, for example registers are sometimes used instead for passing arguments, but referencing the stack seems to be the best way to do this.
1. Data symbols are not yet supported. For example, defining an array of data with a symbol referring to the start of the array isn't supported. This is due to the fact that this tool translates the compiled object code into Golang assembly, at which point most data symbol references in the code have been translated into addresses, which means that simply including the array won't work as it will likely be repositioned in the final binary by go. This translation could be made to work, but it would be quite difficult.
2. The produced Golang assembly currently includes a RET at the end, which means that you shouldn't also include returning instructions (such as `bx lr` for ARM) as the Golang assembler will already insert this information.
3. Supported instructions are translated from native assembly into Golang's supported syntax. For example `mov r2 lr` in native ARM is translated to `MOVW R14, R2` in native plan9 assembly. Currently this is only supported for ARM, but it would be easy to support this on other architecture's using `golang.org/x/arch`.
4. No assembly function flags are currently supported. I eventually hope to solve this by annotating the function's declaration in the go source file. For example to insert the `NOPTR` flag, I think eventually a comment like `// asm2go:noptr` would be included above the function's declaration. Specifying the frame sizes should also probably be supported this way. It would be nice for `asm2go` to dynamically determine the size of the arguments, but this isn't currently implemented.

Furthermore, the assembler must either be specified with the `-as` option, which can be a absolute path or a name on `$PATH`. In the same folder as the assembler must be the executables `strip` and `objdump` must also be available (note that assemblers specified with a prefix such as `arm-linux-gnueabihf-as` works properly; the prefix is resolved to find `arm-linux-gnueabihf-objdump`, etc - this allows cross compiling to work as expected). `strip` is used to remove debugging information from the compiled object file, and `objdump` is used to parse the actual hex instructions that are associated with instructions.

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

The assembly generated uses the WORD feature of Plan9 assembly to translate all unsupported native instructions like such:

```
TEXT ·KeccakF1600(SB), 0, $0-8
    MOVW 0x4(R13), R0  // ldr      r0        [sp #4] 
    MOVW 0x8(R13), R1  // ldr      r1        [sp #8] 
    WORD $0xed2d8b10;  // vpush    {d8-d15}  
    WORD $0xf42007dd;  // vld1.64  {d0}      [r0 :64]!  
    WORD $0xf42027dd;  // vld1.64  {d2}      [r0 :64]!  
    WORD $0xf42047dd;  // vld1.64  {d4}      [r0 :64]!  
...
```

The only required parts are the ARM assembly file (`keccak.s`) and the go declaration file which declares the assembly-implemented function in go. The go file doesn't need anything extra, just the function declaration:

```
package keccak

// go:noescape
// This function is implemented in keccak.s
func KeccakF1600(state *[25]uint64, constants *[24]uint64)

```

## License

This project is licensed under the GPLv3. See LICENSE file for full license.
Copyright 2018 Canonical Ltd.