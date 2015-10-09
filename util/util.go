package util

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"compress/gzip"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/mcuadros/go-version"
	"golang.org/x/sys/unix"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	PRESERVED_CHECKSUM_LENGTH = 64
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "util"})
)

func EncodeData(v interface{}) (*bytes.Buffer, error) {
	param := bytes.NewBuffer(nil)
	j, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if _, err := param.Write(j); err != nil {
		return nil, err
	}
	return param, nil
}

func ExtractUUIDs(names []string, prefix, suffix string) ([]string, error) {
	result := []string{}
	for i := range names {
		f := names[i]
		// Remove additional slash if exists
		f = strings.TrimLeft(f, "/")
		f = strings.TrimPrefix(f, prefix)
		f = strings.TrimSuffix(f, suffix)
		if !ValidateUUID(f) {
			return nil, fmt.Errorf("Invalid name %v was processed to extract UUID with prefix %v surfix %v", names[i], prefix, suffix)
		}
		result = append(result, f)
	}
	return result, nil
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
	if _, err := Execute("rm", []string{fileName}); err != nil {
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
	return result
}

func GetFileChecksum(filePath string) (string, error) {
	output, err := Execute("sha512sum", []string{"-b", filePath})
	if err != nil {
		return "", err
	}
	return strings.Split(string(output), " ")[0], nil
}

func CompressFile(filePath string) error {
	if _, err := Execute("gzip", []string{filePath}); err != nil {
		return err
	}
	return nil
}

func DecompressFile(filePath string) error {
	if _, err := Execute("gunzip", []string{filePath}); err != nil {
		return err
	}
	return nil
}

func CompressDir(sourceDir, targetFile string) error {
	tmpFile := targetFile + ".tmp"
	if _, err := Execute("tar", []string{"cf", tmpFile, "-C", sourceDir, "."}); err != nil {
		return err
	}
	if _, err := Execute("gzip", []string{tmpFile}); err != nil {
		return err
	}
	if _, err := Execute("mv", []string{"-f", tmpFile + ".gz", targetFile}); err != nil {
		return err
	}
	return nil
}

// If sourceFile is inside targetDir, it would be deleted automatically
func DecompressDir(sourceFile, targetDir string) error {
	tmpDir := targetDir + ".tmp"
	if _, err := Execute("rm", []string{"-rf", tmpDir}); err != nil {
		return err
	}
	if err := os.Mkdir(tmpDir, os.ModeDir|0700); err != nil {
		return err
	}
	if _, err := Execute("tar", []string{"xf", sourceFile, "-C", tmpDir}); err != nil {
		return err
	}
	if _, err := Execute("rm", []string{"-rf", targetDir}); err != nil {
		return err
	}
	if _, err := Execute("mv", []string{"-f", tmpDir, targetDir}); err != nil {
		return err
	}
	return nil
}

func Copy(src, dst string) error {
	if _, err := Execute("cp", []string{src, dst}); err != nil {
		return err
	}
	return nil
}

func AttachLoopbackDevice(file string, readonly bool) (string, error) {
	params := []string{"-v", "-f"}
	if readonly {
		params = append(params, "-r")
	}
	params = append(params, file)
	out, err := Execute("losetup", params)
	if err != nil {
		return "", err
	}
	dev := strings.TrimSpace(strings.SplitAfter(string(out[:]), "device is")[1])
	return dev, nil
}

func DetachLoopbackDevice(file, dev string) error {
	output, err := Execute("losetup", []string{dev})
	if err != nil {
		return err
	}
	out := strings.TrimSpace(string(output))
	suffix := "(" + file + ")"
	if !strings.HasSuffix(out, suffix) {
		return fmt.Errorf("Unmatched source file, output %v, suffix %v", out, suffix)
	}
	if _, err := Execute("losetup", []string{"-d", dev}); err != nil {
		return err
	}
	return nil
}

func ValidateUUID(s string) bool {
	return uuid.Parse(s) != nil
}

func CheckUUID(uuid string) error {
	if !ValidateUUID(uuid) {
		return fmt.Errorf("Invalid UUID %v", uuid)
	}
	return nil
}
func ValidateName(name string) bool {
	validName := regexp.MustCompile(`^[0-9a-z_.\-]+$`)
	return validName.MatchString(name)
}

func CheckName(name string) error {
	if name == "" {
		return nil
	}
	if !ValidateName(name) {
		return fmt.Errorf("Invalid name %v", name)
	}
	return nil
}

func ParseSize(size string) (int64, error) {
	if size == "" {
		return 0, nil
	}
	size = strings.ToLower(size)
	readableSize := regexp.MustCompile(`^[0-9.]+[kmgt]$`)
	if !readableSize.MatchString(size) {
		value, err := strconv.ParseInt(size, 10, 64)
		return value, err
	}

	last := len(size) - 1
	unit := string(size[last])
	value, err := strconv.ParseInt(size[:last], 10, 64)
	if err != nil {
		return 0, err
	}

	kb := int64(1024)
	mb := 1024 * kb
	gb := 1024 * mb
	tb := 1024 * gb
	switch unit {
	case "k":
		value *= kb
	case "m":
		value *= mb
	case "g":
		value *= gb
	case "t":
		value *= tb
	default:
		return 0, fmt.Errorf("Unrecongized size value %v", size)
	}
	return value, err
}

func CheckBinaryVersion(binaryName, minVersion string, args []string) error {
	output, err := exec.Command(binaryName, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed version check for %s, due to %s", binaryName, err.Error())
	}
	v := strings.TrimSpace(string(output))
	if version.Compare(v, minVersion, "<") {
		return fmt.Errorf("Minimal require version for %s is %s, detected %s",
			binaryName, minVersion, v)
	}
	return nil
}

func Execute(binary string, args []string) (string, error) {
	output, err := exec.Command(binary, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Failed to execute: %v %v, output %v, error %v", binary, args, string(output), err)
	}
	return string(output), nil
}

func Now() string {
	return time.Now().Format(time.RubyDate)
}

func CompressData(data []byte) (io.ReadSeeker, error) {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		w.Close()
		return nil, err
	}
	w.Close()
	return bytes.NewReader(b.Bytes()), nil
}

func DecompressAndVerify(src io.Reader, checksum string) (io.Reader, error) {
	r, err := gzip.NewReader(src)
	if err != nil {
		return nil, err
	}
	block, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if GetChecksum(block) != checksum {
		return nil, fmt.Errorf("Checksum verification failed for block!")
	}
	return bytes.NewReader(block), nil
}

func GetUUID(v interface{}, key string, required bool, err error) (string, error) {
	uuid, err := GetFlag(v, key, required, err)
	if err != nil {
		return uuid, err
	}
	uuid = strings.ToLower(uuid)
	if !required && uuid == "" {
		return "", nil
	}
	if err := CheckUUID(uuid); err != nil {
		return "", err
	}
	return uuid, nil
}

func GetName(v interface{}, key string, required bool, err error) (string, error) {
	name, err := GetFlag(v, key, required, err)
	if err != nil {
		return name, err
	}
	if !required && name == "" {
		return name, nil
	}
	if err := CheckName(name); err != nil {
		return "", err
	}
	return name, nil
}

func RequiredMissingError(name string) error {
	return fmt.Errorf("Cannot find valid required parameter:", name)
}

func GetFlag(v interface{}, key string, required bool, err error) (string, error) {
	if err != nil {
		return "", err
	}
	value := ""
	switch v := v.(type) {
	default:
		return "", fmt.Errorf("Unexpected type for getLowerCaseFlag")
	case *cli.Context:
		if key == "" {
			value = v.Args().First()
		} else {
			value = v.String(key)
		}
	case map[string]string:
		value = v[key]
	case *http.Request:
		if err := v.ParseForm(); err != nil {
			return "", err
		}
		value = v.FormValue(key)
	}
	if required && value == "" {
		err = RequiredMissingError(key)
	}
	return value, err
}

func UnescapeURL(url string) string {
	// Deal with escape in url inputed from bash
	result := strings.Replace(url, "\\u0026", "&", 1)
	result = strings.Replace(result, "u0026", "&", 1)
	return result
}
