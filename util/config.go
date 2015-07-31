package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func LoadConfig(path, name string, v interface{}) error {
	fileName := filepath.Join(path, name)
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

func SaveConfig(path, name string, v interface{}) error {
	fileName := filepath.Join(path, name)
	j, err := json.Marshal(v)
	if err != nil {
		return err
	}

	tmpFileName := filepath.Join(path, name+".tmp")

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

func ConfigExists(path, name string) bool {
	fileName := filepath.Join(path, name)
	_, err := os.Stat(fileName)
	return err == nil
}

func RemoveConfig(path, name string) error {
	fileName := filepath.Join(path, name)
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
