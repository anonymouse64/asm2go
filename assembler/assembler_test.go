package assembler

import (
	"bytes"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

type instructionTest struct {
	instr           MachineInstruction
	instrByteString string
	arch            string
	tryPlan9        bool
	err             error
	output          string
}

func TestInstructionFormatHex(t *testing.T) {
	tables := []instructionTest{
		{MachineInstruction{
			Command:   "mov",
			Arguments: []string{"r2", "lr"},
			Address:   "0",
		},
			"e1a0200e",
			"arm",
			false,
			nil,
			"WORD $0xe1a0200e; // mov r2 lr",
		},
		{MachineInstruction{
			Command:   "vld1.64",
			Arguments: []string{"{d0}", "[r0 :64]! "},
			Address:   "0",
		},
			"dd0720f4",
			"arm",
			true,
			nil,
			"WORD $0xf42007dd; // vld1.64 {d0} [r0 :64]!",
		},
		{MachineInstruction{
			Command:   "mov",
			Arguments: []string{"r2", "lr"},
			Address:   "0",
		},
			"e1a0200e",
			"arm",
			true,
			nil,
			"MOVW R14, R2 // mov r2 lr",
		},
	}

	// Parse all of the hex strings into the actual byte arrays
	for i := range tables {
		instrBytes, err := hex.DecodeString(tables[i].instrByteString)
		if err != nil {
			t.Errorf("Failed to parse hex string for table %d : %s", i, tables[i].instrByteString)
		}
		tables[i].instr.Bytes = instrBytes
	}

	for _, table := range tables {
		// make a buffer for the tabwriter
		var buf bytes.Buffer
		err := table.instr.WriteOutput(table.arch, &buf, table.tryPlan9)
		tabOutputString := adjustWhitespace(buf.String())
		if err != table.err || tabOutputString != table.output {
			t.Errorf("Unable to make format instruction of (instr=%v, arch=%s, tryPlan9=%t), got: (err=%v,\noutput=%s\n) want: (err=%v,\noutput=%s\n).", table.instr, table.arch, table.tryPlan9, err, tabOutputString, table.err, table.output)
		}
	}
}

// adjustWhitespace replaces any sequence of white space with a single white space in the string
// this simplifies comparing strings that will have formatting in them, etc.
// code from : https://stackoverflow.com/questions/37290693/how-to-remove-redundant-spaces-whitespace-from-a-string-in-golang
func adjustWhitespace(s string) string {
	re_inside_whtsp := regexp.MustCompile(`[\s\p{Zs}]{2,}`)
	innerReplace := re_inside_whtsp.ReplaceAllString(s, " ")
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		} else {
			return r
		}
	}, innerReplace))
}
