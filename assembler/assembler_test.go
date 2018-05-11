package assembler

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
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
			"WORD $0xe1a0200e; \t// mov	r2	lr",
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
			"MOVW R14, R2 	// mov	r2	lr",
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
		tabOutputString := strings.TrimSpace(buf.String())
		if err != table.err || tabOutputString != table.output {
			t.Errorf("Unable to make format instruction of (instr=%v, arch=%s, tryPlan9=%t), got: (output=%s, err=%v) want: (output=%s, err=%v).", table.instr, table.arch, table.tryPlan9, tabOutputString, err, table.output, table.err)
		}
	}
}
