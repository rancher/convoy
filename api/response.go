package api

import (
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"runtime/debug"
	"strings"
)

type ErrorResponse struct {
	Error string
}

type UUIDResponse struct {
	UUID string
}

type VolumeResponse struct {
	UUID        string
	Name        string
	Driver      string
	Size        int64
	MountPoint  string
	CreatedTime string
	Snapshots   map[string]SnapshotResponse
}

type SnapshotResponse struct {
	UUID            string
	VolumeUUID      string `json:",omitempty"`
	VolumeName      string `json:",omitempty"`
	Size            int64  `json:",omitempty"`
	VolumeCreatedAt string `json:",omitempty"`
	Name            string
	CreatedTime     string
}

type BackupsResponse struct {
	Backups map[string]BackupResponse
}

type BackupResponse struct {
	BackupUUID        string
	VolumeUUID        string
	VolumeName        string
	VolumeSize        int64
	VolumeCreatedAt   string
	SnapshotUUID      string
	SnapshotName      string
	SnapshotCreatedAt string
	CreatedTime       string
}

type BackupURLResponse struct {
	URL string
}

func ResponseError(format string, a ...interface{}) {
	response := ErrorResponse{Error: fmt.Sprintf(format, a...)}
	j, err := json.MarshalIndent(&response, "", "\t")
	if err != nil {
		panic(fmt.Sprintf("Failed to generate response for error:", err))
	}
	fmt.Println(string(j[:]))
}

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
	} else if e, ok := v.(error); ok {
		logrus.Errorf(fmt.Sprint(e))
		ResponseError(fmt.Sprint(e))
	} else {
		logrus.Errorf("%s: %s", v, debug.Stack())
		ResponseError("Caught FATAL error: %s", v)
	}
}

func ResponseOutput(v interface{}) ([]byte, error) {
	j, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return nil, err
	}
	return j, nil
}
