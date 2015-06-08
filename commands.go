package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
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

func getLowerCaseFlag(c *cli.Context, name string, required bool, err error) (string, error) {
	if err != nil {
		return "", err
	}
	result := strings.ToLower(c.String(name))
	if required && result == "" {
		err = genRequiredMissingError(name)
	}
	return result, err
}

func getLowerCaseHTTPFlag(value, name string, required bool, err error) (string, error) {
	if err != nil {
		return "", err
	}
	result := strings.ToLower(value)
	if required && result == "" {
		err = genRequiredMissingError(name)
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
	driver := s.StorageDriver
	data, err := driver.Info()
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) notImplemented(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	info := fmt.Sprintf("not implmeneted: %v %v %v", r.Method, r.RequestURI, objs)
	return fmt.Errorf(info)
}
