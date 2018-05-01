package assembler

import (
	"bytes"
	"encoding/hex"
	"testing"
	"text/tabwriter"
)

type instructionTest struct {
	instr           MachineInstruction
	instrByteString string
	arch            string
	err             error
	output          string
}

func TestInstructionFormatHex(t *testing.T) {
	tables := []instructionTest{
		{MachineInstruction{
			Command:   "mov",
			Arguments: []string{"r2", "lr"},
		},
			"e1a0200e",
			"arm",
			nil,
			"    WORD $0xe1a0200e;  // mov r2 lr \n",
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
		w := tabwriter.NewWriter(&buf, 0, 0, 1, ' ', 0)
		err := table.instr.FormatHex(table.arch, w)
		w.Flush()
		tabOutputString := buf.String()
		if err != table.err || tabOutputString != table.output {
			t.Errorf("Unable to make format instruction of (instr=%v, arch=%s), got: (output=%s, err=%v) want: (output=%s, err=%v).", table.instr, table.arch, tabOutputString, err, table.output, table.err)
		}
	}
}
