package util

import (
	"encoding/json"
	"os"
	"strings"
)

func LoadConfig(fileName string, v interface{}) error {
	st, err := os.Stat(fileName)
	if err != nil {
		return err
	}

	file, err := os.Open(fileName)
	if err != nil {
		return err
	}

	defer file.Close()

	data := make([]byte, st.Size())
	_, err = file.Read(data)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, v); err != nil {
		return err
	}
	return nil
}

func SaveConfig(fileName string, v interface{}) error {
	j, err := json.Marshal(v)
	if err != nil {
		return err
	}

	tmpFileName := fileName + ".tmp"

	f, err := os.Create(tmpFileName)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = f.Write(j); err != nil {
		return err
	}

	if _, err = os.Stat(fileName); err == nil {
		err = os.Remove(fileName)
		if err != nil {
			return err
		}
	}

	if err := os.Rename(tmpFileName, fileName); err != nil {
		return err
	}

	return nil
}

func ConfigExists(fileName string) bool {
	_, err := os.Stat(fileName)
	return err == nil
}

func RemoveConfig(fileName string) error {
	if _, err := Execute("rm", []string{"-f", fileName}); err != nil {
		return err
	}
	return nil
}

func ListConfigIDs(path, prefix, suffix string) ([]string, error) {
	out, err := Execute("find", []string{path,
		"-maxdepth", "1",
		"-name", prefix + "*" + suffix,
		"-printf", "%f "})
	if err != nil {
		return []string{}, nil
	}
	if len(out) == 0 {
		return []string{}, nil
	}
	fileResult := strings.Split(strings.TrimSpace(string(out)), " ")
	return ExtractUUIDs(fileResult, prefix, suffix)
}
