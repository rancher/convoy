package dmidecode

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type DMI struct {
	Data map[string]map[string]string
}

func New() *DMI {
	dmi := &DMI{}
	dmi.Data = make(map[string]map[string]string)
	return dmi
}

// Wrapper for FindBin, ExecCmd, ParseDmidecode
func (d *DMI) Run() error {
	bin, findErr := d.FindBin("dmidecode")
	if findErr != nil {
		return findErr
	}

	cmdOutput, cmdErr := d.ExecDmidecode(bin)
	if cmdErr != nil {
		return cmdErr
	}

	if err := d.ParseDmidecode(cmdOutput); err != nil {
		return err
	}

	return nil
}

func (d *DMI) FindBin(binary string) (string, error) {
	locations := []string{"/sbin", "/usr/sbin", "/usr/local/sbin"}

	for _, path := range locations {
		lookup := path + "/" + binary
		fileInfo, err := os.Stat(path + "/" + binary)

		if err != nil {
			continue
		}

		if !fileInfo.IsDir() {
			return lookup, nil
		}
	}

	return "", errors.New(fmt.Sprintf("Unable to find the '%v' binary", binary))
}

func (d *DMI) ExecDmidecode(binary string) (string, error) {
	cmd := exec.Command(binary)

	output, err := cmd.Output()

	if err != nil {
		return "", err
	}

	return string(output), nil
}

// Gross; maybe there is a cleaner way to get this done via multiline regex
func (d *DMI) ParseDmidecode(output string) error {
	// Each record is separated by double newlines
	splitOutput := strings.Split(output, "\n\n")

	for _, record := range splitOutput {
		recordElements := strings.Split(record, "\n")

		// Entries with less than 3 lines are incomplete/inactive; skip them
		if len(recordElements) < 3 {
			continue
		}

		handleRegex, _ := regexp.Compile("^Handle\\s+(.+),\\s+DMI\\s+type\\s+(\\d+),\\s+(\\d+)\\s+bytes$")
		handleData := handleRegex.FindStringSubmatch(recordElements[0])

		if len(handleData) == 0 {
			continue
		}

		dmiHandle := handleData[1]

		d.Data[dmiHandle] = make(map[string]string)
		d.Data[dmiHandle]["DMIType"] = handleData[2]
		d.Data[dmiHandle]["DMISize"] = handleData[3]

		// Okay, we know 2nd line == name
		d.Data[dmiHandle]["DMIName"] = recordElements[1]

		inBlockElement := ""
		inBlockList := ""

		// Loop over the rest of the record, gathering values
		for i := 2; i < len(recordElements); i++ {
			// Check whether we are inside a \t\t block
			if inBlockElement != "" {
				inBlockRegex, _ := regexp.Compile("^\\t\\t(.+)$")
				inBlockData := inBlockRegex.FindStringSubmatch(recordElements[i])

				if len(inBlockData) > 0 {
					if len(inBlockList) == 0 {
						inBlockList = inBlockData[1]
					} else {
						inBlockList = inBlockList + "\t\t" + inBlockData[1]
					}
					d.Data[dmiHandle][inBlockElement] = inBlockList
					continue
				} else {
					// We are out of the \t\t block; reset it again, and let
					// the parsing continue
					inBlockElement = ""
				}
			}

			recordRegex, _ := regexp.Compile("\\t(.+):\\s+(.+)$")
			recordData := recordRegex.FindStringSubmatch(recordElements[i])

			// Is this the line containing handle identifier, type, size?
			if len(recordData) > 0 {
				d.Data[dmiHandle][recordData[1]] = recordData[2]
				continue
			}

			// Didn't match regular entry, maybe an array of data?
			recordRegex2, _ := regexp.Compile("\\t(.+):$")
			recordData2 := recordRegex2.FindStringSubmatch(recordElements[i])

			if len(recordData2) > 0 {
				// This is an array of data - let the loop know we are inside
				// an array block
				inBlockElement = recordData2[1]
				continue
			}
		}
	}

	if len(d.Data) == 0 {
		return errors.New("Unable to parse 'dmidecode' output")
	}

	return nil
}

// Generic map lookup method
func (d *DMI) GenericSearchBy(param, value string) (map[string]string, error) {
	if len(d.Data) == 0 {
		return nil, errors.New("DMI data is empty; make sure to .Run() first")
	}

	for _, v := range d.Data {
		if v[param] == value {
			return v, nil
		}
	}

	return make(map[string]string), nil
}

// Search for a specific DMI record by name
func (d *DMI) SearchByName(name string) (map[string]string, error) {
	return d.GenericSearchBy("DMIName", name)
}

// Search for a specific DMI record by its type
func (d *DMI) SearchByType(id int) (map[string]string, error) {
	return d.GenericSearchBy("DMIType", strconv.Itoa(id))
}
