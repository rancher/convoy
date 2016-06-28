package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rancher/convoy/api"
)

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

func writeStringResponse(w http.ResponseWriter, s string) error {
	log.Debugln("Response: ", s)
	_, err := w.Write([]byte(s))
	return err
}

func (s *daemon) doInfo(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	_, err = w.Write([]byte(fmt.Sprint("{\n\"General\": ")))
	if err != nil {
		return err
	}

	data, err := api.ResponseOutput(&s.daemonConfig)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	if err != nil {
		return err
	}
	for _, driver := range s.ConvoyDrivers {
		if _, err := w.Write([]byte(fmt.Sprintf(",\n\"%v\": ", driver.Name()))); err != nil {
			return err
		}

		info, err := driver.Info()
		if err != nil {
			return err
		}
		data, err = api.ResponseOutput(info)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		if err != nil {
			return err
		}
	}

	if _, err := w.Write([]byte(fmt.Sprint("\n}"))); err != nil {
		return err
	}

	return nil
}
