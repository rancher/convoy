package server

import (
	"encoding/json"
	"fmt"
	"github.com/rancher/rancher-volume/api"
	"net/http"
	"path/filepath"
)

const (
	CONFIGFILE = "rancher-volume.cfg"
)

func getConfigFileName(root string) string {
	return filepath.Join(root, CONFIGFILE)
}

func getCfgName() string {
	return CONFIGFILE
}

func decodeRequest(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func sendResponse(w http.ResponseWriter, v interface{}) error {
	resp, err := api.ResponseOutput(v)
	if err != nil {
		return err
	}
	_, err = w.Write(resp)
	if err != nil {
		return err
	}
	return nil
}

func writeResponseOutput(w http.ResponseWriter, v interface{}) error {
	output, err := api.ResponseOutput(v)
	if err != nil {
		return err
	}
	log.Debugln("Response: ", string(output))
	_, err = w.Write(output)
	return err
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
