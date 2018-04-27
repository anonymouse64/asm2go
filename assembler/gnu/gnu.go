package gnu

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/anonymouse64/asm2go/assembler"
)

// GnuAssembler implements Assembler interface and works with gnu "as" (aka "gas") assembler
type GnuAssembler struct {
	// The assembler executable itself - this should always be an absolute path
	AsExecutable string
	// The architecture to compile for
	Arch string
	// for cross-compilers such as arm-linux-gnueabihf-as, we can find other tools (such as objdump) automatically
	// by looking in the same folder as asExecutable and prepending the prefix to whatever tool we are looking
	// in the example for arm-linux-gnueabihf-as, prefix will be "arm-linux-gnueabihi-"
	Prefix string
	// the folder where tools such as gcc, as, objdump, strip etc. should all be found
	// this should always be equal to filepath.Split(g.asExecutable)
	BinToolsFolder string
}

func (g GnuAssembler) toolExecutable(name string) string {
	return filepath.Join(g.BinToolsFolder, g.Prefix+name)
}

func (g GnuAssembler) objdump() string {
	return g.toolExecutable("objdump")
}

// Architecture returns the architecture of the this GNU assembler
func (g GnuAssembler) Architecture() string {
	return g.Arch
}

// AssembleToMachineCode takes an assembly file with options and returns a corresponding compiled object file, and a
// assembly listing file
func (g GnuAssembler) AssembleToMachineCode(file string, asOpts []string) (string, string, error) {
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
	asCmd := exec.Command(g.AsExecutable, args...)
	cmb, err := asCmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("error assembling (%v) : \n%s", err, string(cmb[:]))
	}

	// Now strip all debug information from the file, which probably isn't present, but if it is
	// it will mess up the parsing of the assembly source alongside the instruction bytes
	stripCmd := exec.Command(g.toolExecutable("strip"), "--strip-debug", objFile)
	stripCmb, err := stripCmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("error stripping debug info from object file (%v) : \n%s", err, string(stripCmb[:]))
	}

	return objFile, lisFile, nil
}

// ParseObjectSymbols takes in an object file and returns a list of all symbols from that object file
func (g GnuAssembler) ParseObjectSymbols(objectFile string) ([]assembler.Symbol, error) {
	// To get all the object symbols from the object file, we use objdump with the -t option to display symbol names
	// and the C option demangles C++ names
	cmd := exec.Command(g.objdump(), "-t", "-C", objectFile)
	cmb, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error processing object file %s (%v) : \n%s", objectFile, err, string(cmb[:]))
	}
	strOutput := string(cmb[:])

	// Find the first occurrence of "SYMBOL TABLE:"
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

// This regex matches the hex address of an instruction, the binary of the instruction itself, and then the corresponding instruction
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
func (g GnuAssembler) ProcessMachineCodeToInstructions(objectFile string, syms map[string]assembler.Symbol) (map[string][]assembler.MachineInstruction, error) {
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
	symMachInstrs := make(map[string][]assembler.MachineInstruction)
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
				symMachInstrs[sym] = append(symMachInstrs[sym], assembler.MachineInstruction{
					Address:           instMatches[1],
					Bytes:             decodedBytes,
					RawInstruction:    instMatches[3],
					InstructionString: rawInstructions[0],
					Comment:           strings.TrimSpace(commentString),
					Command:           opcodeMatches[0][1],
					Arguments:         formattedArgs,
				})
			}
		}
	}

	return symMachInstrs, nil
}

func processObjdumpTable(tableRows []string) ([]assembler.Symbol, error) {
	var symbols []assembler.Symbol
	var err error
	for _, line := range tableRows {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}
		var sym assembler.Symbol
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
func parseFlagString(sym *assembler.Symbol, flagString string) error {
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
