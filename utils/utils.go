package utils

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	PRESERVED_CHECKSUM_LENGTH = 64
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

	var f *os.File
	if _, err = os.Stat(fileName); err == nil {
		err = os.Remove(fileName)
		if err != nil {
			return err
		}
	}
	f, err = os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = f.Write(j); err != nil {
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
	if err := exec.Command("rm", "-f", fileName).Run(); err != nil {
		return err
	}
	return nil
}

func ListConfigIDs(path, prefix, suffix string) []string {
	out, err := exec.Command("find", path,
		"-maxdepth", "1",
		"-name", prefix+"*"+suffix,
		"-printf", "%f ").Output()
	if err != nil {
		return []string{}
	}
	if len(out) == 0 {
		return []string{}
	}
	fileResult := strings.Split(strings.TrimSpace(string(out)), " ")
	result := make([]string, len(fileResult))
	for i := range fileResult {
		f := fileResult[i]
		f = strings.TrimPrefix(f, prefix)
		result[i] = strings.TrimSuffix(f, suffix)
	}
	return result
}

func MkdirIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, os.ModeDir|0700); err != nil {
			return err
		}
	}
	return nil
}

func GetChecksum(data []byte) string {
	checksumBytes := sha512.Sum512(data)
	checksum := hex.EncodeToString(checksumBytes[:])[:PRESERVED_CHECKSUM_LENGTH]
	return checksum
}

func LockFile(fileName string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func UnlockFile(fileName string) error {
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_UN); err != nil {
		return err
	}
	if err := exec.Command("rm", fileName).Run(); err != nil {
		return err
	}
	return nil
}

func SliceToMap(slices []string) map[string]string {
	result := map[string]string{}
	for _, v := range slices {
		pair := strings.Split(v, "=")
		if len(pair) != 2 {
			return nil
		}
		result[pair[0]] = pair[1]
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
