// asm2go - a utility for automatically generating golang assembly wrappers from complete native assembly
// functions

// run with go run asm2go.go -as arm-linux-gnueabihf-as -file keccak.s -as-opts -march=armv7-a -as-opts -mfpu=neon-vfpv4

package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	"github.com/kr/pretty"
)

type arrayFlags []string

func (i *arrayFlags) String() string {
	return ""
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var assemblerOptions arrayFlags

// MachineInstruction represents an individual machine instruction as found in a binary
// executable, etc.
type MachineInstruction struct {
	// The raw assembly instruction as parsed from the output
	RawInstruction string
	// The bytes corresponding to the actual machine instruction assembled
	Bytes []byte
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
	// The address of the instruction as reported by objdump
	Address string
}

// FunctionDeclaration represents a function declaration as found in a go source file
// It is used primarily to parse information from the go declaration for an assembly function
// and then use that information to fill in the information needed in the plan9 assembly function
// declaration
type FunctionDeclaration struct {
	// The name of the function
	Name string
	// The names of each of the arguments
	ArgumentNames []string
	// The type of each argument as a reflect.Type, because that's easier to work with than the ast types
	ArgumentTypes []reflect.Type
	// The size of each argument in bytes - note that if the input is a static array of a fixed size then this count
	// will be the size of each element * number of elements, but if it is a slice, then this will just be 3 int64's for
	// the start of the slice, the length and the capacity of the slice
	ArgumentSizes []uintptr
	ResultNames   []string
	ResultTypes   []reflect.Type
	ResultSizes   []uintptr
}

// Assembler is a generic assembler implementation interface
// i.e. this interface is implemented for GNU assembler (aka gas) with gnuAssembler, etc.
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

type gnuAssembler struct {
	// The assembler executable itself - this should always be an absolute path
	asExecutable string
	// The architecture to compile for
	arch string
	// for cross-compilers such as arm-linux-gnueabihf-as, we can find other tools (such as objdump) automatically
	// by looking in the same folder as asExecutable and prepending the prefix to whatever tool we are looking
	// in the example for arm-linux-gnueabihf-as, prefix will be "arm-linux-gnueabihi-"
	prefix string
	// the folder where tools such as gcc, as, objdump, strip etc. should all be found
	// this should always be equal to filepath.Split(g.asExecutable)
	binToolsFolder string
}

func (g gnuAssembler) toolExecutable(name string) string {
	return filepath.Join(g.binToolsFolder, g.prefix+name)
}

func (g gnuAssembler) objdump() string {
	return g.toolExecutable("objdump")
}

func (g gnuAssembler) AssembleToMachineCode(file string, asOpts []string) (string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	// Get the filenames to use for this assembly
	_, fileBaseName := filepath.Split(file)
	lisFile := filepath.Join(cwd, "asm2go-"+fileBaseName+".lis")
	objFile := filepath.Join(cwd, "asm2go-"+fileBaseName+".obj")

	args := []string{
		"-o",
		objFile,
		fmt.Sprintf("-aln=%s", lisFile),
		file,
	}

	// Add any additional assembler options that might be necessary
	args = append(args, asOpts...)

	// Run the assembler to compile the file into object code
	asCmd := exec.Command(g.asExecutable, args...)
	cmb, err := asCmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("error assembling (%v) : \n%s", err, string(cmb[:]))
	}

	// Now strip all debug information from the file, which probably isn't present, but if it is
	// it will mess up the parsing of the assembly source alongside the instruction bytes
	stripCmd := exec.Command(filepath.Join(g.binToolsFolder, g.prefix+"strip"), "--strip-debug", objFile)
	stripCmb, err := stripCmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("error stripping debug info from object file (%v) : \n%s", err, string(stripCmb[:]))
	}

	return objFile, lisFile, nil
}

// ParseObjectSymbols takes in an object file and returns a list of all symbols from that object file
func (g gnuAssembler) ParseObjectSymbols(objectFile string) ([]Symbol, error) {
	// To get all the object symbols from the object file, we use objdump with the -t option to display symbol names
	// and the C option demangles C++ names
	cmd := exec.Command(g.objdump(), "-t", "-C", objectFile)
	cmb, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error processing object file %s (%v) : \n%s", objectFile, err, string(cmb[:]))
	}
	strOutput := string(cmb[:])

	// Find the first occurence of "SYMBOL TABLE:"
	symbolTableStart := strings.Index(strOutput, "SYMBOL TABLE:")
	if symbolTableStart == -1 {
		return nil, fmt.Errorf("error processing objdump output: %v", cmb)
	}

	// Split everything by newlines and remove the first line, which is "SYMBOL TABLE:"
	tableRows := strings.Split(strOutput[symbolTableStart:], "\n")
	if len(tableRows) < 2 {
		return nil, fmt.Errorf("error processing objdump output: %v", cmb)
	}
	tableRows = tableRows[1:]

	// Now actually process all of the rows into Symbol's
	return processObjdumpTable(tableRows)
}

func deleteSpace(r rune) rune {
	if unicode.IsSpace(r) {
		return -1
	}
	return r
}

// This regex matches the hex address of an instrucion, the binary of the instruction itself, and then the corresponding instruction
// as 3 subgroups
var instructionRegex = regexp.MustCompile(`(?m)^(?:\s*)([0-9a-f]+):(?:\s*)([0-9a-f ]+)\t(.+)$`)

// This regex matches an opcode of letters, numbers and the ".", and all possible arguments as 2 subgroups
var opcodeArgsRegex = regexp.MustCompile(`(?m)(^[a-zA-z0-9.]+)(?:\s*)(.*)$`)

// This regex matches the end of a set of instructions associated with a symbol
// a more readable version of this regex would be simply a check for the next line that is "\t..."
// or the empty string after calling strings.TrimSpace
var symbolEndRegex = regexp.MustCompile(`(?m)(^((\t\.\.\.)|[ \t]*)$)|(^$)`)

// ProcessMachineCodeToInstructions takes in an object file and a map of symbol names -> Symbol that are to be processed
// and returns a map of symbol name -> machine instructions corresponding to that symbol
func (g gnuAssembler) ProcessMachineCodeToInstructions(objectFile string, syms map[string]Symbol) (map[string][]MachineInstruction, error) {
	// First, we use objdump on the object file to get a listing of the disassembled source
	cmd := exec.Command(g.objdump(), "-S", "-C", "-w", objectFile)
	cmb, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error processing object file %s (%v) : \n%s", objectFile, err, string(cmb[:]))
	}
	lines := strings.Split(string(cmb[:]), "\n")

	// With the source file, we need to find the first line in the output that starts with "FFFFFFF <SYMBOL_NAME>:"
	// (FFFFFFF being some hex address) as that is the start of the disassembly for the specified symbols
	// then find the end of the instructions for that symbol identified by either the first blank line after the start
	// oy by "\t..." which is displayed for padding 0's that may be added to the end of the symbol's instructions
	symInstrStrings := make(map[string][]string)
	for sym := range syms {
		var start int
		var end int
		// We have to generate this regex each time, as we include the name of the symbol in the regex
		symbolStartRegex := regexp.MustCompile(fmt.Sprintf(`(?m)^[0-9a-f]+ <%s>:`, sym))

		for index, line := range lines {
			loc := symbolStartRegex.FindStringIndex(line)
			if len(loc) == 2 {
				// found the start, now look for the end
				start = index
				for index2, line := range lines[index:] {
					loc := symbolEndRegex.FindStringIndex(line)
					if len(loc) == 2 {
						// the ending isn't just index2, it's index2 + the length of the start
						end = index2 + start
						break
					}
				}
				break
			}
		}
		// the range starts at start+1 to drop the "FFFFFFF <SYMBOL_NAME>:""
		symInstrStrings[sym] = lines[start+1 : end]
	}

	// Now that we have all the instruction lines, we need to parse each line into a MachineInstruction
	symMachInstrs := make(map[string][]MachineInstruction)
	for sym, instrStrings := range symInstrStrings {
		// Loop over each instruction, parsing it into a MachineInstruction
		for _, instrString := range instrStrings {
			for _, instMatches := range instructionRegex.FindAllStringSubmatch(instrString, -1) {
				// In the second group delete all whitespace to join all hex bytes together into a single string
				// Then we decode it into an actual byte slice
				decodedBytes, err := hex.DecodeString(strings.Map(deleteSpace, instMatches[2]))
				if err != nil {
					return nil, err
				}

				// The RawInstruction occurs in the 3rd element of match and may have a
				// comment after it, usually automatically generated for symbols that have been resolved to a hex address
				// so we split it by the ";" which is the comment character, then we can split the instruction itself
				// into opcodes / arguments
				var commentString string
				rawInstructions := strings.SplitN(instMatches[3], ";", 2)
				if len(rawInstructions) == 1 {
					commentString = ""
				} else {
					commentString = rawInstructions[1]
				}

				// Now find the instruction and the opcodes using the regex which reports the opcode
				// as the first subgroup and all arguments (if any) as the second group which will always exist
				// but sometimes may be the empty string
				opcodeMatches := opcodeArgsRegex.FindAllStringSubmatch(rawInstructions[0], -1)
				if len(opcodeMatches) == 0 {
					return nil, fmt.Errorf("error: invalid instruction format: %s", instrString)
				}

				// Split the arguments by a comma and trim off all whitespace
				instrArgs := strings.Split(opcodeMatches[0][2], ",")
				formattedArgs := make([]string, len(instrArgs))
				for index, instrArg := range instrArgs {
					formattedArgs[index] = strings.TrimSpace(instrArg)
				}

				// Finally build up the instruction and add it into the map
				symMachInstrs[sym] = append(symMachInstrs[sym], MachineInstruction{
					Address:        instMatches[1],
					Bytes:          decodedBytes,
					RawInstruction: instMatches[3],
					Comment:        strings.TrimSpace(commentString),
					Command:        opcodeMatches[0][1],
					Arguments:      formattedArgs,
				})
			}
		}
	}

	return symMachInstrs, nil
}

func processObjdumpTable(tableRows []string) ([]Symbol, error) {
	var symbols []Symbol
	var err error
	for _, line := range tableRows {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}
		var sym Symbol
		// First handle the symbol value / address
		out := strings.SplitN(trimmedLine, " ", 2)
		if len(out) < 2 {
			return nil, fmt.Errorf("error processing objdump row (line is incorrectly formatted) : %s", line)
		}

		sym.ValueAddressField, err = strconv.ParseUint(out[0], 16, 64)
		if err != nil {
			return nil, err
		}

		// Now handle the flags string, which will always be length of 7 char's
		restLine := out[1]
		if len(restLine) < 8 {
			return nil, fmt.Errorf("error processing objdump row (line is missing flag column) : %s", line)
		}
		err = parseFlagString(&sym, restLine[:7])
		if err != nil {
			return nil, err
		}

		// Drop the flag string from the row and process the rest of the line as the section, alignment/size field and the name
		// Note that the separator between the section and the alignment/size field is a tab, while everywhere else is a space
		// hence the duplicated strings.Split
		cols := strings.Split(restLine[8:], "\t")
		if len(cols) < 2 {
			return nil, fmt.Errorf("error processing objdump row (line is too short) : %s", line)
		}
		cols = append([]string{cols[0]}, strings.SplitN(cols[1], " ", 2)...)
		if len(cols) < 3 {
			return nil, fmt.Errorf("error processing objdump row (line is too short) : %s", line)
		}
		sym.Section = cols[0]
		sym.AlignmentSizeField, err = strconv.ParseUint(cols[1], 16, 64)
		if err != nil {
			return nil, err
		}
		sym.Name = cols[2]

		symbols = append(symbols, sym)
	}

	return symbols, nil
}

// parseFlagString works on the 2nd column of `objdump -t`
// documentation on this column used from here : http://manpages.ubuntu.com/manpages/xenial/en/man1/objdump.1.html
func parseFlagString(sym *Symbol, flagString string) error {
	if sym == nil || len(flagString) == 0 {
		return fmt.Errorf("invalid arguments : sym=%+v, flagString=%+v ", sym, flagString)
	}
	switch flagString[0] {
	case 'l':
		sym.Local = true
	case 'g':
		sym.Global = true
	case 'u':
		sym.UniqueGlobal = true
	case '!':
		sym.Global = true
		sym.Local = true
	case ' ':
		break
	default:
		return fmt.Errorf("invalid flag at position 0 : %c", flagString[0])
	}

	switch flagString[1] {
	case 'w':
		sym.Weak = true
	case ' ':
		break
	default:
		return fmt.Errorf("invalid flag at position 1 : %c", flagString[1])
	}

	switch flagString[2] {
	case 'C':
		sym.Constructor = true
	case ' ':
		break
	default:
		return fmt.Errorf("invalid flag at position 2 : %c", flagString[2])
	}

	switch flagString[3] {
	case 'W':
		sym.Warning = true
	case ' ':
		break
	default:
		return fmt.Errorf("invalid flag at position 3 : %c", flagString[3])
	}

	switch flagString[4] {
	case 'I':
		sym.IndirectReference = true
	case 'i':
		sym.RelocationProcessingFunction = true
	case ' ':
		break
	default:
		return fmt.Errorf("invalid flag at position 4 : %c", flagString[4])
	}

	switch flagString[5] {
	case 'd':
		sym.Debugging = true
	case 'D':
		sym.Dynamic = true
	case ' ':
		break
	default:
		return fmt.Errorf("invalid flag at position 5 : %c", flagString[5])
	}

	switch flagString[6] {
	case 'F':
		sym.Function = true
	case 'f':
		sym.File = true
	case 'O':
		sym.Object = true
	case ' ':
		break
	default:
		return fmt.Errorf("invalid flag at position 6 : %c", flagString[6])
	}

	return nil
}

// makeAssembler uses the user-specified assemblerName + assemblerFile to fill in details about the assembler
// to use for assembling the program
func makeAssembler(assemblerName string, assemblerFile string) (Assembler, error) {
	// First see if we have the name of this assembler, in which case we can just try to find a corresponding assembler file
	var err error
	var assemblerExecName string
	_, assemblerExec := filepath.Split(assemblerFile)
	arch := runtime.GOARCH
	switch assemblerName {
	case "":
		// We don't have the name, so look in the file, which should be an absolute file
		switch {
		case strings.Contains(assemblerFile, "yasm"):
			// TODO: implement yasm support
			return gnuAssembler{}, fmt.Errorf("%s is not supported yet\n", assemblerFile)
		case assemblerExec == "as":
			// native "as" treat as gas
			fallthrough
		case strings.Contains(assemblerFile, "gcc") || strings.Contains(assemblerFile, "gnu"):
			// Determine the prefix for this assembler - make sure that the assembler ends in "as"
			binToolsFolder, prefix := filepath.Split(assemblerFile)
			if strings.HasSuffix(assemblerFile, "as") {
				// Drop the last 2 characters and use that as the prefix
				prefix = prefix[:len(prefix)-2]
			} else {
				prefix = ""
			}
			// Use gas assembler, check what architecture
			if strings.Contains(assemblerFile, "arm") {
				// TODO: handle arm64 properly
				return gnuAssembler{
					asExecutable:   assemblerFile,
					arch:           "arm",
					prefix:         prefix,
					binToolsFolder: binToolsFolder,
				}, nil
			}
			return gnuAssembler{
				asExecutable:   assemblerFile,
				arch:           arch,
				prefix:         prefix,
				binToolsFolder: binToolsFolder,
			}, nil
		case strings.Contains(assemblerFile, "armcc"):
			// TODO: implement armcc
			fallthrough
		default:
			return gnuAssembler{}, fmt.Errorf("%s is not supported yet\n", assemblerFile)
		}
	case "arm-linux-gnueabihf-as":
		arch = "arm"
		assemblerExecName = "arm-linux-gnueabihf-as"
		fallthrough
	case "gas":
		if assemblerExecName == "" {
			assemblerExecName = "as"
		}
		var executable string
		// If the file path wasn't specified look for it
		if assemblerFile == "" {
			executable, err = exec.LookPath(assemblerExecName)
			if err != nil {
				return gnuAssembler{}, err
			}
		} else {
			executable = assemblerFile
		}
		binToolsFolder, prefix := filepath.Split(executable)
		prefix = prefix[:len(prefix)-2]
		return gnuAssembler{
			asExecutable:   executable,
			arch:           arch,
			prefix:         prefix,
			binToolsFolder: binToolsFolder,
		}, nil
	default:
		return gnuAssembler{}, fmt.Errorf("%s is not supported yet\n", assemblerName)
	}
}

// parseGoLangFileForFuncDecls will parse a golang source file looking for suitable
// assembly implemented function declarations and return any found functions
// the map is of the function name to the declaration struct
func parseGoLangFileForFuncDecls(goSrc string) (map[string]FunctionDeclaration, error) {

	const src = `package main

import "fmt"
import "strings"

func main() {
    hello := "Hello"
    world := "World"
    words := []string{hello, world}
    SayHello(words)
}

// SayHello says Hello
func SayHello(words []string) {
    fmt.Println(joinStrings(words))
}

// joinStrings joins strings
func joinStrings(words []string) string {
    return strings.Join(words, ", ")
}

// asm is an assembly implemented function
// it does things that work in assembly language

type helloWorld struct {
	h int
	w string
}

func asm() [5]struct{h int}
`

	// Create the AST by parsing src.
	fset := token.NewFileSet() // positions are relative to fset
	f, err := parser.ParseFile(fset, goSrc, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// Create an ast.CommentMap from the ast.File's comments.
	// This helps keeping the association between comments
	// and AST nodes.
	cmap := ast.NewCommentMap(fset, f, f.Comments)

	funcDecls := make(map[string]FunctionDeclaration)

	// Inspect the AST and print all function declarations that have nil body's
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			// If the body of this function is nil, then it's an assembly implemented function we are interested in
			if x.Body == nil {
				decl := FunctionDeclaration{}
				decl.Name = x.Name.Name

				//funcArgs := x.Type.Params.List
				funcResults := x.Type.Results
				if funcResults == nil {
					fmt.Println(funcName, "has no results")
				} else {
					for _, res := range funcResults.List {
						fmt.Printf("res type %T\n", res.Type)
						switch z := res.Type.(type) {
						case *ast.ArrayType:
							if z.Len == nil {
								fmt.Printf("result type is slice of: %#v\n", z)
							} else {
								if length, ok := z.Len.(*ast.BasicLit); ok {
									fmt.Printf("z.Elt is %#v\n", z.Elt)
									switch elemType := z.Elt.(type) {
									case *ast.StructType:
										if elemType.Incomplete {
											fmt.Printf("result type is array of incomplete structs with fields %#v and length %#v\n", elemType.Fields, length.Value)
										} else {
											fmt.Printf("result type is array of type struct with %d fields and length %#v\n", len(elemType.Fields.List), length.Value)
										}
									case *ast.Ident:
										fmt.Printf("result type is array of type %#v and length %#v\n", elemType.Name, length.Value)
									}
								} else {
									// Some error with this function declaration
									return true
								}
							}
						case *ast.Ident:
							fmt.Printf("%s result type: %#v\n", funcName, z)
						}
					}
				}
				var funcComments string
				for _, comment := range cmap.Filter(x).Comments() {
					funcComments += comment.Text()
				}
				fmt.Printf("%s:\t%s\n", fset.Position(n.Pos()), x.Name.Name)
				fmt.Println(funcComments)
			}
		}
		// we want to walk the entire AST, so always return true here
		return true
	})

	return funcDecls, nil
}

// generate Plan9Assembly takes in a go declaration file, the output file and a mapping of symbol names to the corresponding instructions
// It generates the wrapper function text around the assembly code by parsing information from the assoociated golang function in
// the declaration file. This means that the name of the golang function must match exactly the name of the symbols in the compiled object file
// Additionally, argument information isn't parsed to do anything with the instructions itself, but is used to populate the go comment above
// the function implementation itself. If a symbol is deemed "interesting" (see comments in main() for explicit explanation of this creiterion),
// but doesn't have a corresponding golang function, then no such export comment is generated for it and that symbol/function is assumed to be
// just available inside the assembly file
func generatePlan9Assembly(goDeclarationFile string, outputFile string, syms map[string][]MachineInstruction) error {

	// First make sure the goDeclarationFile exists
	if _, err := os.Stat(goDeclarationFile); err != nil {
		// doesn't exist or can't be opened
		return err
	}

	// always show what command generated this file and also always include the textflag.h include file for
	// stuff like NOPTR, etc.
	outputStr := fmt.Sprintf(`// generated by asm2go %s DO NOT EDIT
	#include "textflag.h"`, strings.Join(os.Args[1:], " "))

	// For each symbol in the list, which should only be functions, other types aren't yet supported
	// generate the function signature
	signature := `%s
TEXT ·%s(SB), 0, $200-8`

	for sym, instrs := range syms {
		// TODO: handle arguments correctly
		fmt.Sprintf(signature, sym, "", sym)
	}

	return nil
}

func main() {
	// Setup flags
	flag.Var(&assemblerOptions, "as-opts", "Assembler options to use")
	assembler := flag.String("as", "gas", "assembler to use")
	fileOpt := flag.String("file", "", "file to assemble")
	goFileOpt := flag.String("gofile", "", "go file with function declarations")
	outputFile := flag.String("out", "", "output file to place data in (empty uses stdout)")
	flag.Parse()

	file := *fileOpt

	// Check if the file exists
	_, err := os.Stat(file)
	switch {
	case err != nil:
		fmt.Printf("error checking file: %v\n", err)
		os.Exit(1)
	}

	// Check the assembler option
	assemblerString := strings.ToLower(*assembler)
	assemblerOnPath, _ := exec.LookPath(assemblerString)

	var as Assembler
	// First handle named assemblers, then check if the assembler specified is a file
	if assemblerString == "gas" || assemblerString == "as" || assemblerString == "gcc" {
		as, err = makeAssembler("gas", "")
	} else if assemblerString == "yasm" {
		// TODO
	} else if assemblerString == "armcc" {
		// TODO
	} else if _, err := os.Stat(*assembler); err == nil {
		// assembler is a valid file path
		as, err = makeAssembler("", *assembler)
	} else if _, err := os.Stat(assemblerOnPath); err == nil {
		// assembler is a file that exists on the $PATH
		as, err = makeAssembler("", assemblerOnPath)
	} else {
		fmt.Printf("assembler %s not supported\n", *assembler)
		os.Exit(1)
	}
	if err != nil {
		fmt.Printf("error finding assembler: %v\n", err)
		os.Exit(1)
	}

	// Now compile to object file + assembly listing using the assembly options specified by
	// the user
	objectFile, _, err := as.AssembleToMachineCode(file, assemblerOptions)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Now parse the object file to get all the symbols
	syms, err := as.ParseObjectSymbols(objectFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Iterate through the symbols and find the "useful" ones
	// Note to future maintainer : these criterion were somehwat arbitrary chosen and
	// may need to be changed, but currently is just:
	// - Not a Debugging symbol
	// - Not a Warning symbol
	// - Not a File symbol
	// - Section is not "*UND*" (i.e. it's not in an undefined section, i.e. another object file)
	usefulSymbolMap := make(map[string]Symbol)
	var usefulSymbolNames []string
	for _, sym := range syms {
		if !sym.Debugging && !sym.Warning && !sym.File && sym.Section != "*UND*" {
			usefulSymbolNames = append(usefulSymbolNames, sym.Name)
			usefulSymbolMap[sym.Name] = sym
		}
	}

	fmt.Printf("useful symbols are : %#v\n", pretty.Formatter(usefulSymbolNames))

	symsToInstructions, err := as.ProcessMachineCodeToInstructions(objectFile, usefulSymbolMap)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Printf("symbols + instructions: %#v\n", pretty.Formatter(symsToInstructions))

	// Now that we have a complete symbol -> instructions map we can begin generating go/plan9 assembly code for
	// all of the functions
	err := generatePlan9Assembly(outputFile, symsToInstructions)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
