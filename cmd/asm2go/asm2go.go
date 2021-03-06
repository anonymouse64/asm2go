// asm2go - a utility for automatically generating golang assembly wrappers from complete native assembly
// functions

package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/anonymouse64/asm2go/assembler"
	"github.com/anonymouse64/asm2go/assembler/gnu"
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
	ArgumentSizes   []uintptr
	ResultNames     []string
	ResultTypes     []reflect.Type
	ResultSizes     []uintptr
	SignatureString string
	DocComments     string
}

// makeAssembler uses the user-specified assemblerName + assemblerFile to fill in details about the assembler
// to use for assembling the program
func makeAssembler(assemblerName string, assemblerFile string) (assembler.Assembler, error) {
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
			return assembler.InvalidAssembler(), fmt.Errorf("%s is not supported yet", assemblerFile)
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
				return gnu.GnuAssembler{
					AsExecutable:   assemblerFile,
					Arch:           "arm",
					Prefix:         prefix,
					BinToolsFolder: binToolsFolder,
				}, nil
			} else if strings.Contains(assemblerFile, "aarch64") {
				return gnu.GnuAssembler{
					AsExecutable:   assemblerFile,
					Arch:           "arm64",
					Prefix:         prefix,
					BinToolsFolder: binToolsFolder,
				}, nil
			}
			return gnu.GnuAssembler{
				AsExecutable:   assemblerFile,
				Arch:           arch,
				Prefix:         prefix,
				BinToolsFolder: binToolsFolder,
			}, nil
		case strings.Contains(assemblerFile, "armcc"):
			// TODO: implement armcc
			fallthrough
		default:
			return assembler.InvalidAssembler(), fmt.Errorf("%s is not supported yet", assemblerFile)
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
				return assembler.InvalidAssembler(), err
			}
		} else {
			executable = assemblerFile
		}
		binToolsFolder, prefix := filepath.Split(executable)
		prefix = prefix[:len(prefix)-2]
		return gnu.GnuAssembler{
			AsExecutable:   executable,
			Arch:           arch,
			Prefix:         prefix,
			BinToolsFolder: binToolsFolder,
		}, nil
	default:
		return assembler.InvalidAssembler(), fmt.Errorf("%s is not supported yet", assemblerName)
	}
}

// getStringFromFilePosition gets the associated string from a file given a start and end position
func getStringFromFilePosition(fset *token.FileSet, start, end token.Pos) (string, error) {
	// Check that the start comes before the end
	if start > end {
		return "", fmt.Errorf("error: invalid positions : %v -> %v", start, end)
	}

	// Make sure that the two positions are for the same file
	startFile := fset.File(start)
	endFile := fset.File(end)

	if endFile == nil || startFile == nil {
		return "", fmt.Errorf("error: start or end positions are nil")
	}

	if startFile != endFile {
		return "", fmt.Errorf("error: start + end are not in the same file (start=%#v, end=%#v)", startFile, endFile)
	}

	absoluteStart := fset.Position(start)
	absoluteEnd := fset.Position(end)

	// Check that the file exists
	if _, err := os.Stat(absoluteStart.Filename); err != nil {
		return "", err
	}

	// Open the file for reading
	f, err := os.Open(absoluteStart.Filename)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)

	// Note: line numbers from token.Position is 1-indexed
	lineNumber := 1

	// corner case where the start + end are on the same line
	if absoluteStart.Line == absoluteEnd.Line {
		// Scan up to the specifiied line number
		for lineNumber = 1; scanner.Scan() && lineNumber < absoluteStart.Line; lineNumber++ {
		}

		// Make sure we actually read the required number of lines, otherwise fail
		if lineNumber != absoluteStart.Line {
			fmt.Println(lineNumber)
			fmt.Println(absoluteStart.Line)
			return "", fmt.Errorf("error: line %d doesn't exist in file %s", absoluteStart.Line, absoluteStart.Filename)
		}
		// Otherwise we are on the desired line, so make sure that the end column is within this line
		line := scanner.Text()
		if absoluteEnd.Column-1 <= len(line) {
			return line[absoluteStart.Column-1 : absoluteEnd.Column-1], nil
		} else {
			// Line too short
			fmt.Println(absoluteStart.Column, absoluteEnd.Column, len(line), line)
			return "", fmt.Errorf("error: line %d of %s too short", lineNumber, absoluteStart.Filename)
		}
	}

	// General case - start + end on different lines
	// Start scanning up to the start line number
	var text string

	for ; scanner.Scan(); lineNumber++ {
		if lineNumber == absoluteStart.Line {
			// then we found the start - ensure that the column number for the start is within this line
			line := scanner.Text()
			if absoluteStart.Column <= len(line) {
				// Column number is 1-indexed so subtract 1 from it for the position in the string slice
				text = line[absoluteStart.Column-1:]
				break
			} else {
				return "", fmt.Errorf("error: line %d of %s too short", lineNumber, absoluteStart.Filename)
			}
		}
	}

	// Now scan up to the end line number, adding all text up to the end column
	for ; scanner.Scan(); lineNumber++ {
		if lineNumber == absoluteEnd.Line {
			// then we found the end - ensure that the column number for the start is within this line
			line := scanner.Text()
			if absoluteEnd.Column <= len(line) {
				// Column number is 1-indexed so subtract 1 from it for the position in the string slice
				text += line[absoluteStart.Column-1:]
				break
			} else {
				return "", fmt.Errorf("error: line %d of %s too short", lineNumber, absoluteStart.Filename)
			}
		} else {
			text += scanner.Text()
		}
	}

	return text, nil
}

// parseGoLangFileForFuncDecls will parse a golang source file looking for suitable
// assembly implemented function declarations and return any found functions
// the map is of the function name to the declaration struct
func parseGoLangFileForFuncDecls(goSrc string) (map[string]FunctionDeclaration, error) {

	// Create an AST by parsing the go file
	fset := token.NewFileSet()
	// Ensure that we also parse comments into the file set
	f, err := parser.ParseFile(fset, goSrc, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// Create an ast.CommentMap from the ast.File's comments.
	// This helps keeping the association between comments
	// and AST nodes.
	cmap := ast.NewCommentMap(fset, f, f.Comments)

	funcDecls := make(map[string]FunctionDeclaration)

	// Walk the AST and look for all FuncDecl's that don't have a body.
	ast.Inspect(f, func(n ast.Node) bool {
		switch function := n.(type) {
		case *ast.FuncDecl:
			// If the body of this function is nil, then it's an assembly implemented function we are interested in
			if function.Body == nil {
				decl := FunctionDeclaration{}
				decl.Name = function.Name.Name

				// TODO: this is largely unimplemented, due to the large number of
				// different cases that need to be handled for the args/results

				// Iterate over the function arguments to gather information on the function args
				for _, arg := range function.Type.Params.List {
					switch z := arg.Type.(type) {
					case *ast.ArrayType:
						if z.Len == nil {
							// arg is a slice
							return true
						} else {
							if _, ok := z.Len.(*ast.BasicLit); ok {
								switch elemType := z.Elt.(type) {
								case *ast.StructType:
									if elemType.Incomplete {
										// fmt.Printf("arg type is array of incomplete structs with fields %#v and length %#v\n", elemType.Fields, length.Value)
									} else {
										// fmt.Printf("arg type is array of type struct with %d fields and length %#v\n", len(elemType.Fields.List), length.Value)
									}
									return true
								case *ast.Ident:
									// fmt.Printf("arg type is array of type %#v and length %#v\n", elemType.Name, length.Value)
									return true
								}
							} else {
								// Some error with this function declaration - just move onto the next ast node
								return true
							}
						}
					case *ast.Ident:
					}
				}

				// Next do a similar check on the results of the function
				// Note that the Results can be nil : https://golang.org/pkg/go/ast/#FuncType
				if function.Type.Results != nil {
					for _, res := range function.Type.Results.List {
						// Switch on the type of result
						switch z := res.Type.(type) {
						case *ast.ArrayType:
							if z.Len == nil {
								// res is a slice
								return true
							} else {
								// result is an array of a specific length
								if _, ok := z.Len.(*ast.BasicLit); ok {
									switch elemType := z.Elt.(type) {
									case *ast.StructType:
										// Then this result is returning a list of structs
										// TODO: support returning array of structs
										if elemType.Incomplete {
											// fmt.Printf("arg type is array of incomplete structs with fields %#v and length %#v\n", elemType.Fields, length.Value)
										} else {
											// fmt.Printf("arg type is array of type struct with %d fields and length %#v\n", len(elemType.Fields.List), length.Value)
										}
										return true
									case *ast.Ident:
										// This result is returning a concrete type of array of - determine what kind of type the array is

									}
								} else {
									// Some error with this function declaration - just move onto the next ast node
									return true
								}
							}
						case *ast.Ident:
						}
					}
				}

				// To get associated documentation comments for this function, we don't use function.Doc, as that won't pick up comments that have
				// a newline separating the
				var funcComments string
				for _, comment := range cmap.Filter(function).Comments() {
					funcComments += comment.Text()
				}
				decl.DocComments = funcComments

				// Get the full signature of this function from the source file using the pos + end
				// note that this works because there is no body - so this entire declaration consists of just the
				// signature
				decl.SignatureString, err = getStringFromFilePosition(fset, function.Pos(), function.End())
				if err != nil {
					fmt.Println(err)
					return true
				}

				// Put this function declaration into the map
				funcDecls[decl.Name] = decl
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
func generatePlan9Assembly(goDeclarationFile, outputFile, arch string, syms map[string][]assembler.MachineInstruction) error {

	// First make sure the goDeclarationFile exists
	if goDeclarationFile == "" {
		return fmt.Errorf("error: gofile must be specified")
	}
	if _, err := os.Stat(goDeclarationFile); err != nil {
		// doesn't exist or can't be opened
		return err
	}

	// Now parse function declarations for the declaration file
	decls, err := parseGoLangFileForFuncDecls(goDeclarationFile)
	if err != nil {
		return err
	}

	// Setup the output mechanism - we use tabbed writing for prettier formatted assembly
	// If the outputFile is an empty string, we just print to stdout
	var output io.Writer
	if outputFile == "" {
		output = os.Stdout
	} else {
		f, err := os.Create(outputFile)
		if err != nil {
			return err
		}

		defer f.Close()
		output = f
	}
	w := tabwriter.NewWriter(output, 0, 0, 1, ' ', 0)

	// Add a header to the file generated to show what command generated this file and also
	// always include the textflag.h include file for stuff like NOSPLIT, NOPTR, etc.
	fmt.Fprintf(w, `// Generated by asm2go %s DO NOT EDIT
#include "textflag.h"

`, strings.Join(os.Args[1:], " "))

	// For each symbol in the list, which should only be functions, other types aren't yet supported
	// add the assembly TEXT signature
	for sym, instrs := range syms {
		funcDecl, ok := decls[sym]
		if !ok {
			// Then this symbol doesn't have a corresponding go function that calls it, so we can just insert it into the file
			// as a basic TEXT with reported stack size of 0 and no flags
			// TODO implement...
			return fmt.Errorf("error: symbol %s not found in go file declaration : %s", sym, goDeclarationFile)
		}

		// Calculate the total number of bytes for the args + results
		var totalBytes uintptr
		for _, argBytes := range funcDecl.ArgumentSizes {
			totalBytes += argBytes
		}
		for _, resBytes := range funcDecl.ResultSizes {
			totalBytes += resBytes
		}

		// TODO: get the golang function signature and include it in the assembly signature comment

		// Format the function signature
		fmt.Fprintf(w,
			`%s
TEXT ·%s(SB), %s, $%d-8
`,
			"// "+funcDecl.SignatureString,
			sym,
			// TODO: handle flags here
			"0",
			totalBytes,
		)

		// NOTE: for arm64, currently the disassembler doesn't sync with the assembler
		// and so we shouldn't try to translate supported op codes because the dissassembler
		// produces syntax that the assembler doesn't understand
		trySupportedTranslation := true
		if arch == "arm64" {
			trySupportedTranslation = false
		}

		// Now output all of the instructions for this symbol
		for _, instr := range instrs {
			err := instr.WriteOutput(arch, w, trySupportedTranslation)
			if err != nil {
				return err
			}
		}

		// Finally for this symbol append a RET to the end
		// this handles all returns in all architectures
		fmt.Fprintln(w, "    RET")
	}

	// Flush all output
	w.Flush()

	return nil
}

func main() {
	// Setup flags
	flag.Var(&assemblerOptions, "as-opts", "Assembler options to use")
	assemblerOpt := flag.String("as", "gas", "assembler to use")
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
	assemblerString := strings.ToLower(*assemblerOpt)
	assemblerOnPath, _ := exec.LookPath(assemblerString)

	var as assembler.Assembler
	// First handle named assemblers, then check if the assembler specified is a file
	if assemblerString == "gas" || assemblerString == "as" || assemblerString == "gcc" {
		as, err = makeAssembler("gas", "")
	} else if assemblerString == "yasm" {
		// TODO
	} else if assemblerString == "armcc" {
		// TODO
	} else if _, statErr := os.Stat(*assemblerOpt); statErr == nil {
		// assembler is a valid file path
		as, err = makeAssembler("", *assemblerOpt)
	} else if _, statErr := os.Stat(assemblerOnPath); statErr == nil {
		// assembler is a file that exists on the $PATH
		as, err = makeAssembler("", assemblerOnPath)
	} else {
		fmt.Printf("assembler %s not supported\n", *assemblerOpt)
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
	// - Section is not "*ABS*" (i.e. it is a symbol associated with a particular section)
	usefulSymbolMap := make(map[string]assembler.Symbol)
	var usefulSymbolNames []string
	for _, sym := range syms {
		if !sym.Debugging && !sym.Warning && !sym.File && sym.Section != "*UND*" && sym.Section != "*ABS*" {
			usefulSymbolNames = append(usefulSymbolNames, sym.Name)
			usefulSymbolMap[sym.Name] = sym
		}
	}

	// fmt.Printf("useful symbols are : %#v\n", pretty.Formatter(usefulSymbolNames))

	symsToInstructions, err := as.ProcessMachineCodeToInstructions(objectFile, usefulSymbolMap)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// fmt.Printf("symbols + instructions: %#v\n", pretty.Formatter(symsToInstructions))

	// Now that we have a complete symbol -> instructions map we can begin generating go/plan9 assembly code for
	// all of the functions
	err = generatePlan9Assembly(*goFileOpt, *outputFile, as.Architecture(), symsToInstructions)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
