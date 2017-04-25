package dmidecode

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

const (
	fakeBinary string = "time4soup"
	testDir    string = "test_data"
)

func TestFindBin(t *testing.T) {
	dmi := New()

	if _, err := dmi.FindBin("time4soup"); err == nil {
		t.Error("Should not be able to find obscure binary")
	}

	bin, findErr := dmi.FindBin("dmidecode")
	if findErr != nil {
		t.Errorf("Should be able to find dmidecode. Error: %v", findErr)
	}

	_, statErr := os.Stat(bin)

	if statErr != nil {
		t.Errorf("Should be able to lookup found file. Error: %v", statErr)
	}
}

func TestExecDmidecode(t *testing.T) {
	dmi := New()

	if _, err := dmi.ExecDmidecode("/bin/" + fakeBinary); err == nil {
		t.Errorf("Should get an error trying to execute a fake binary. Error: %v", err)
	}

	bin, findErr := dmi.FindBin("dmidecode")
	if findErr != nil {
		t.Errorf("Should be able to find binary. Error: %v", findErr)
	}

	output, execErr := dmi.ExecDmidecode(bin)

	if execErr != nil {
		t.Errorf("Should not get errors executing '%v'. Error: %v", bin, execErr)
	}

	if len(output) == 0 {
		t.Errorf("Output should not be empty")
	}
}

func TestParseDmidecode(t *testing.T) {
	dmi := New()

	bin, findErr := dmi.FindBin("dmidecode")
	if findErr != nil {
		t.Errorf("Should be able to find binary. Error: %v", findErr)
	}

	output, execErr := dmi.ExecDmidecode(bin)

	if execErr != nil {
		t.Errorf("Should not get errors executing '%v'. Error: %v", bin, execErr)
	}

	if err := dmi.ParseDmidecode(output); err != nil {
		t.Error("Should not receive an error after parsing dmidecode output")
	}

	if len(dmi.Data) == 0 {
		t.Error("Parsed data structure should have more than 0 entries")
	}

	files, globErr := filepath.Glob(testDir + "/*")
	if globErr != nil {
		t.Errorf("Should not receive errors during '%v' glob. Error: %v", testDir, globErr)
	}

	for _, file := range files {
		// Let's clear it out, each iteration (just in case)
		dmi.Data = make(map[string]map[string]string)

		data, readErr := ioutil.ReadFile(file)
		if readErr != nil {
			t.Errorf("Should not receive errors while reading contents of '%v'. Error: %v", file, readErr)
		}

		if err := dmi.ParseDmidecode(string(data)); err != nil {
			t.Errorf("Should not get errors while parsing '%v'. Error:", file, err)
		}

		if len(dmi.Data) == 0 {
			t.Errorf("Data length should be larger than 0 after reading '%v'", file)
		}
	}
}

func TestRun(t *testing.T) {
	dmi := New()

	if err := dmi.Run(); err != nil {
		t.Errorf("Run() should not return any errors. Error: %v", err)
	}
}

func TestSearchBy(t *testing.T) {
	dmi := New()

	if _, err := dmi.SearchByName("System Information"); err == nil {
		t.Error("Should have received an error when Search ran prior to .Run()")
	}

	if _, err := dmi.SearchByType(1); err == nil {
		t.Error("Should have received an error when Search ran prior to .Run()")
	}

	if _, err := dmi.GenericSearchBy("DMIName", "System Information"); err == nil {
		t.Error("Should have received an error when Search ran prior to .Run()")
	}

	if err := dmi.Run(); err != nil {
		t.Errorf("Run() should not return any errors. Error: %v", err)
	}

	byNameData, byNameErr := dmi.SearchByName("System Information")
	if byNameErr != nil {
		t.Errorf("Shouldn't receive errors when searching by name. Error: %v", byNameErr)
	}

	if len(byNameData) == 0 {
		t.Error("Returned data should have more than 0 records")
	}

	byTypeData, byTypeErr := dmi.SearchByType(1)
	if byTypeErr != nil {
		t.Errorf("Shouldn't receive errors when searching by name. Error: %v", byTypeErr)
	}

	if len(byTypeData) == 0 {
		t.Error("Returned data should have more than 0 records")
	}

	genericData, genericErr := dmi.GenericSearchBy("DMIName", "System Information")
	if genericErr != nil {
		t.Errorf("Shouldn't receive errors when searching by name. Error: %v", genericErr)
	}

	if len(genericData) == 0 {
		t.Error("Returned data should have more than 0 records")
	}
}
