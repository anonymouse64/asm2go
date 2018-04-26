package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

type assemblerTest struct {
	as   Assembler
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
		{gnuAssembler{
			asExecutable:   gasExec,
			arch:           runtime.GOARCH,
			binToolsFolder: gasExecFolder,
			prefix:         "",
		},
			nil,
			"gas",
			"",
		},
		{gnuAssembler{
			asExecutable:   gasExec,
			arch:           runtime.GOARCH,
			binToolsFolder: gasExecFolder,
			prefix:         "",
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
				{gnuAssembler{
					asExecutable:   armGas,
					arch:           "arm",
					binToolsFolder: armExecFolder,
					prefix:         "arm-linux-gnueabihf-",
				},
					nil,
					"",
					armGas,
				},
				{gnuAssembler{
					asExecutable:   armGas,
					arch:           "arm",
					binToolsFolder: armExecFolder,
					prefix:         "arm-linux-gnueabihf-",
				},
					nil,
					"arm-linux-gnueabihf-as",
					"",
				},
			}...)
	}

	for _, table := range tables {
		as, err := makeAssembler(table.name, table.file)
		if !compareAsgnuAssemblers(as, table.as) || err != table.err {
			t.Errorf("Unable to make assembler of (name=%s, file=%s), got: (as=%#v, err=%v) want: (as=%#v, err=%v).", table.name, table.file, as, err, table.as, table.err)
		}
	}

}

func compareAsgnuAssemblers(as Assembler, g Assembler) bool {
	// cast g to a gnuAssembler
	if g2, ok := g.(gnuAssembler); ok {
		// cast the assembler to a gnuAssembler
		if gnuAs, ok := as.(gnuAssembler); ok {
			// make sure the fields match
			return gnuAs.arch == g2.arch && gnuAs.asExecutable == g2.asExecutable && gnuAs.binToolsFolder == g2.binToolsFolder && gnuAs.prefix == g2.prefix
		}
	}
	// it's not a gnuAssembler, so return false
	return false
}
