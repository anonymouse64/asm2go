package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/anonymouse64/asm2go/assembler"
	"github.com/anonymouse64/asm2go/assembler/gnu"
)

type assemblerTest struct {
	as   assembler.Assembler
	err  error
	name string
	file string
}

func TestMakeAssembler(t *testing.T) {
	// Make sure we can find as on this system
	gasExec, err := exec.LookPath("as")
	if err != nil {
		t.Errorf("gnu as not available on the system, failing : %v.", err)
	}
	t.Logf("testing with gnu as : %s\n", gasExec)
	gasExecFolder, _ := filepath.Split(gasExec)
	tables := []assemblerTest{
		{gnu.GnuAssembler{
			AsExecutable:   gasExec,
			Arch:           runtime.GOARCH,
			BinToolsFolder: gasExecFolder,
			Prefix:         "",
		},
			nil,
			"gas",
			"",
		},
		{gnu.GnuAssembler{
			AsExecutable:   gasExec,
			Arch:           runtime.GOARCH,
			BinToolsFolder: gasExecFolder,
			Prefix:         "",
		},
			nil,
			"",
			gasExec,
		},
	}

	armGas, err := exec.LookPath("arm-linux-gnueabihf-as")
	if err != nil {
		t.Logf("arm gnu as not available on the system, not testing")
	} else {
		t.Logf("testing with arm gnu as : %s\n", armGas)
		armExecFolder, _ := filepath.Split(armGas)
		tables = append(tables,
			[]assemblerTest{
				{gnu.GnuAssembler{
					AsExecutable:   armGas,
					Arch:           "arm",
					BinToolsFolder: armExecFolder,
					Prefix:         "arm-linux-gnueabihf-",
				},
					nil,
					"",
					armGas,
				},
				{gnu.GnuAssembler{
					AsExecutable:   armGas,
					Arch:           "arm",
					BinToolsFolder: armExecFolder,
					Prefix:         "arm-linux-gnueabihf-",
				},
					nil,
					"arm-linux-gnueabihf-as",
					"",
				},
			}...)
	}

	for _, table := range tables {
		as, err := makeAssembler(table.name, table.file)
		if !compareAsGnuAssemblers(as, table.as) || err != table.err {
			t.Errorf("Unable to make assembler of (name=%s, file=%s), got: (as=%#v, err=%v) want: (as=%#v, err=%v).", table.name, table.file, as, err, table.as, table.err)
		}
	}
}

func compareAsGnuAssemblers(as assembler.Assembler, g assembler.Assembler) bool {
	// cast g to a GnuAssembler
	if g2, ok := g.(gnu.GnuAssembler); ok {
		// cast the assembler to a GnuAssembler
		if gnuAs, ok := as.(gnu.GnuAssembler); ok {
			// make sure the fields match
			return gnuAs.Arch == g2.Arch && gnuAs.AsExecutable == g2.AsExecutable && gnuAs.BinToolsFolder == g2.BinToolsFolder && gnuAs.Prefix == g2.Prefix
		}
	}
	// it's not a GnuAssembler, so return false
	return false
}
