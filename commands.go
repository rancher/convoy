package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/util"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
)

var (
	infoCmd = cli.Command{
		Name:   "info",
		Usage:  "information about volmgr",
		Action: cmdInfo,
	}
)

func getConfigFileName(root string) string {
	return filepath.Join(root, CONFIGFILE)
}

func getCfgName() string {
	return CONFIGFILE
}

func genRequiredMissingError(name string) error {
	return fmt.Errorf("Cannot find valid required parameter:", name)
}

func getUUID(v interface{}, key string, required bool, err error) (string, error) {
	uuid, err := getLowerCaseFlag(v, key, required, err)
	if err != nil {
		return uuid, err
	}
	if !required && uuid == "" {
		return uuid, nil
	}
	if !util.ValidateUUID(uuid) {
		return "", fmt.Errorf("Invalid UUID %v", uuid)
	}
	return uuid, nil
}

func getName(v interface{}, key string, required bool, err error) (string, error) {
	name, err := getLowerCaseFlag(v, key, required, err)
	if err != nil {
		return name, err
	}
	if !required && name == "" {
		return name, nil
	}
	if !util.ValidateName(name) {
		return "", fmt.Errorf("Invalid name %v", name)
	}
	return name, nil
}

func getLowerCaseFlag(v interface{}, key string, required bool, err error) (string, error) {
	if err != nil {
		return "", err
	}
	value := ""
	switch v := v.(type) {
	default:
		return "", fmt.Errorf("Unexpected type for getLowerCaseFlag")
	case *cli.Context:
		value = v.String(key)
	case map[string]string:
		value = v[key]
	case *http.Request:
		if err := v.ParseForm(); err != nil {
			return "", err
		}
		value = v.FormValue(key)
	}
	result := strings.ToLower(value)
	if required && result == "" {
		err = genRequiredMissingError(key)
	}
	return result, err
}

func cmdInfo(c *cli.Context) {
	if err := doInfo(c); err != nil {
		panic(err)
	}
}

func doInfo(c *cli.Context) error {
	rc, _, err := client.call("GET", "/info", nil, nil)
	if err != nil {
		return err
	}
	defer rc.Close()

	b, err := ioutil.ReadAll(rc)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func (s *Server) doInfo(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	var err error
	_, err = w.Write([]byte(fmt.Sprint("{\n\"General\" : ")))
	if err != nil {
		return err
	}

	data, err := api.ResponseOutput(&s.Config)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(fmt.Sprint(",\n\"Driver\" : "))); err != nil {
		return err
	}

	driver := s.StorageDriver
	data, err = driver.Info()
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(fmt.Sprint("\n}"))); err != nil {
		return err
	}

	return nil
}
