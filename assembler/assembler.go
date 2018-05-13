package assembler

import (
	"encoding/binary"
	"fmt"
	"io"

	"golang.org/x/arch/arm/armasm"
)

const (
	unrecognizedInstr = `instruction %s not supported in plan9`
	unsupportedArch   = `architecture %s not supported`
)

// MachineInstruction represents an individual machine instruction as found in a binary
// executable, etc.
type MachineInstruction struct {
	// The raw assembly instruction as parsed from the output
	RawInstruction string
	// The instruction string without any comments
	InstructionString string
	// The bytes corresponding to the actual machine instruction assembled
	Bytes []byte
	// The endianness of the instruction bytes
	BytesEndianness binary.ByteOrder
	// The command (or opcode) of the instruction
	Command string
	// The arguments for the opcode - can be nil if command has no arguments
	Arguments []string
	// The line number this instruction was found on - note not currently implemented
	LineNumber uint64
	// Any comment found in the objdump output of the machine instruction - note that this won't correspond to
	// the comment included in original source (even if the original source was in assembly)
	// but rather any automatically generated comments such as the hex value of particular constants that were
	// translated from labels, i.e. "jmp MYLABEL" might get translated into "jmp #16 ; 0x10" if MYLABEL gets put at
	// address 0x10
	Comment string
	// The address of the instruction (i.e. the PC)
	Address uint64
}

// Assembler is a generic assembler implementation interface
// i.e. this interface is implemented for GNU assembler (aka gas) with GnuAssembler, etc.
// Currently only implemented for GNU assembler, but armcc + yasm are on the TODO list
type Assembler interface {
	// AssembleToMachineCode takes an assembly source file, and assembler options and
	// returns an object code file and assembly listing file suitable for parsing in ProcessMachineCodeToInstructions
	AssembleToMachineCode(string, []string) (string, string, error)
	// ParseObjectSymbols will take in a file and return all symbols defined from that file
	ParseObjectSymbols(string) ([]Symbol, error)
	// ProcessMachineCodeToInstructions will take a map of symbol names -> symbols (determined from
	// processing ParseObjectSymbols return value) and should produce a map of those symbols to their
	// respective instructions
	ProcessMachineCodeToInstructions(string, map[string]Symbol) (map[string][]MachineInstruction, error)
	// Architecture returns the architecture that this compiler runs for
	Architecture() string
}

// Symbol is a entry in the symbol table of an object file
type Symbol struct {
	// Global is whether this symbol has the "g" flag set
	Global bool
	// UniqueGlobal is whether this symbol has the "u" flag set
	UniqueGlobal bool
	// Local is whether this symbol has the "l" flag set
	Local bool
	// Weak is whether this symbol has the "w" flag set
	Weak bool
	// Constructor is whether this symbol has the "C" flag set
	Constructor bool
	// Warning is whether this symbol has the "W" flag set
	Warning bool
	// IndirectReference is whether this symbol has the "I" flag set
	IndirectReference bool
	// RelocationProcessingFunction is whether this symbol has the "i" flag set
	RelocationProcessingFunction bool
	// Debugging is whether this symbol has the "d" flag set
	Debugging bool
	// Dynamic is whether this symbol has the "D" flag set
	Dynamic bool
	// Function is whether this symbol has the "F" flag set
	Function bool
	// File is whether this symbol has the "f" flag set
	File bool
	// Object is whether this symbol has the "O" flag set
	Object bool
	// Name is the name of the symbol (5th column in `objdump -t output`)
	Name string
	// Section is what section the symbol is in (3rd column in `objdump -t output`)
	Section string
	// AlignmentSizeField is the 4th column in `objdump -t output`
	AlignmentSizeField uint64
	// ValueAddressField is the 1st column in `objdump -t output`
	ValueAddressField uint64
}

type invalidAssembler struct{}

func (i invalidAssembler) AssembleToMachineCode(string, []string) (string, string, error) {
	return "", "", fmt.Errorf("unimplemented assembler")
}

func (i invalidAssembler) ParseObjectSymbols(string) ([]Symbol, error) {
	return nil, fmt.Errorf("unimplemented assembler")
}

func (i invalidAssembler) ProcessMachineCodeToInstructions(string, map[string]Symbol) (map[string][]MachineInstruction, error) {
	return nil, fmt.Errorf("unimplemented assembler")
}

func (i invalidAssembler) Architecture() string {
	return "invalid"
}

// InvalidAssembler returns an Assembler that doesn't work or do anything - useful for returning errors...
func InvalidAssembler() Assembler {
	return invalidAssembler{}
}

func (instr MachineInstruction) errIsUnsupported(err error, arch string) bool {
	return err.Error() == fmt.Sprintf(unrecognizedInstr, instr.Command) || err.Error() == fmt.Sprintf(unsupportedArch, arch)
}

// WriteOutput formats an instruction for golang compatibility using unsupported opcode syntax.
// See https://golang.org/doc/asm#unsupported_opcodes for more details
// tryTranslate controls whether or not to attempt to translate this instruction to Golang syntax
// and output that instead
func (instr MachineInstruction) WriteOutput(arch string, w io.Writer, tryTranslate bool) error {
	// Write out the indentation for this instruction
	fmt.Fprintf(w, "    ")

	// Switch on the method to use for outputting this instruction
	switch {
	case tryTranslate:
		err := instr.writePlan9Supported(arch, w)
		// if there was no error, exit the switch, otherwise fallback on
		// using unsupported opcode syntax
		if err == nil {
			break
		} else if !instr.errIsUnsupported(err, arch) {
			// error isn't an unsupported error, so return this and fail for this instruction
			return err
		}
		fallthrough
	default:
		instr.writePlan9Unsupported(arch, w)
	}

	// Now we add the actual instructions as a new column for each command/argument
	fmt.Fprintf(w, "// %s\t", instr.Command)
	for _, arg := range instr.Arguments {
		fmt.Fprintf(w, "%s\t", arg)
	}

	fmt.Fprintln(w)

	return nil

}

func reverseEndianness(byteSlice []byte) {
	for i, j := 0, len(byteSlice)-1; i < j; i, j = i+1, j-1 {
		byteSlice[i], byteSlice[j] = byteSlice[j], byteSlice[i]
	}
}

func (instr MachineInstruction) writePlan9Unsupported(arch string, w io.Writer) error {
	// First check whether the architecture specified is 32-bit or 64-bit
	// default to 64-bit
	maxBits := 64
	switch arch {
	case "amd64",
		"arm64":
		maxBits = 64
	case "arm":
		maxBits = 32
	}

	// Calculate the prefixes to use based on the number of bits
	var prefixes []string
	var lengths []int
	if maxBits == 64 {
		prefixes = []string{
			"QUAD $0x%02x%02x%02x%02x%02x%02x%02x%02x; \t",
			"LONG $0x%02x%02x%02x%02x; \t",
			"WORD $0x%02x%02x; \t",
			"BYTE $0x%02x; \t",
		}
		lengths = []int{
			8,
			4,
			2,
			1,
		}
	} else if maxBits == 32 {
		// TODO : check other 32-bit architecures to see what isa length they support...
		// To my knowledge, ARM, PowerPC, and MIPS all only support fixed width 32-bit instructions,
		// but others may allow/more
		// However, on 386, we also have LONG, but it's not clear from the plan 9 assembler reference what size
		// LONG is : https://9p.io/sys/doc/asm.html
		// So for now, just assume that every 32-bit architecture only allows WORD's and BYTE's
		prefixes = []string{
			"WORD $0x%02x%02x%02x%02x; \t",
			"BYTE $0x%02x; \t",
		}
		lengths = []int{
			4,
			1,
		}
	}

	// Iterate over the various lengths to insert, inserting as many of the bytes as we can
	// for each size
	opcodes := instr.Bytes
	for i, byteLen := range lengths {
		// While we have more opcodes than the current size, add that size
		for len(opcodes) >= byteLen {
			// This trick let's us use the variadic argument to Fprintf - we put all of
			// the opcodes into a []interface{}, rather than use the []byte directly
			// Note that using the []byte directly doesn't work because you can't cast a []type
			// into an []interface{} without looping over each element of the []type, casting each
			// element into an interface{} because an interface{} contains more than just the underlying
			// object
			args := make([]interface{}, byteLen)
			for i, opcode := range opcodes[:byteLen] {
				args[i] = opcode
			}
			// For some reason the plan9 assembler puts down data for 32 bit architectures in the order they appear
			// but for 64-bit architecture's swaps the endianness, so for 64-bit we need to reverse the endianness of the bytes
			// them into the array
			if maxBits == 64 && instr.BytesEndianness == binary.LittleEndian {
				for i, j := 0, len(args)-1; i < j; i, j = i+1, j-1 {
					args[i], args[j] = args[j], args[i]
				}
			}

			fmt.Fprintf(w, prefixes[i], args...)

			// Drop these bytes for next time
			opcodes = opcodes[byteLen:]
		}
	}

	return nil
}

func (instr MachineInstruction) writePlan9Supported(arch string, w io.Writer) error {
	switch arch {
	case "arm":
		// the arm decoder expects the bytes in little endian
		instrBytes := instr.Bytes
		reverseEndianness(instrBytes)
		// to translate this machine instruction into plan9 assembly, first see if it can be decoded
		goInstr, err := armasm.Decode(instrBytes, armasm.ModeARM)
		if err != nil {
			// Then we couldn't decode this instruction and we should
			// use the WORD method
			return fmt.Errorf(unrecognizedInstr, instr.Command)
		}

		fmt.Fprintf(w, "%s \t", armasm.GoSyntax(goInstr, instr.Address, nil, nil))
	default:
		return fmt.Errorf(unsupportedArch, arch)
	}

	return nil
}
