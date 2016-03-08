package api

import (
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"runtime"
	"runtime/debug"
	"strings"
)

type ErrorResponse struct {
	Error string
}

type VolumeResponse struct {
	Name        string
	Driver      string
	MountPoint  string
	CreatedTime string
	DriverInfo  map[string]string
	Snapshots   map[string]SnapshotResponse
}

type SnapshotResponse struct {
	Name            string
	VolumeName      string `json:",omitempty"`
	VolumeCreatedAt string `json:",omitempty"`
	CreatedTime     string
	DriverInfo      map[string]string
}

type BackupURLResponse struct {
	URL string
}

// ResponseError would generate a error information in JSON format for output
func ResponseError(format string, a ...interface{}) {
	response := ErrorResponse{Error: fmt.Sprintf(format, a...)}
	j, err := json.MarshalIndent(&response, "", "\t")
	if err != nil {
		panic(fmt.Sprintf("Failed to generate response for error:", err))
	}
	fmt.Println(string(j[:]))
}

// ResponseLogAndError would log the error before call ResponseError()
func ResponseLogAndError(v interface{}) {
	if e, ok := v.(*logrus.Entry); ok {
		e.Error(e.Message)
		oldFormatter := e.Logger.Formatter
		logrus.SetFormatter(&logrus.JSONFormatter{})
		s, err := e.String()
		logrus.SetFormatter(oldFormatter)
		if err != nil {
			ResponseError(err.Error())
			return
		}
		// Cosmetic since " would be escaped
		ResponseError(strings.Replace(s, "\"", "'", -1))
	} else {
		e, isErr := v.(error)
		_, isRuntimeErr := e.(runtime.Error)
		if isErr && !isRuntimeErr {
			logrus.Errorf(fmt.Sprint(e))
			ResponseError(fmt.Sprint(e))
		} else {
			logrus.Errorf("Caught FATAL error: %s", v)
			debug.PrintStack()
			ResponseError("Caught FATAL error: %s", v)
		}
	}
}

// ResponseOutput would generate a JSON format byte array of object for output
func ResponseOutput(v interface{}) ([]byte, error) {
	j, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return nil, err
	}
	return j, nil
}
